package app

import (
	"math"
	"testing"
)

func TestPlausiblePowerW(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"idle", 0, 0},
		{"typical laptop", 63.9, 63.9},
		{"ultra studio ceiling", 900, 900},
		{"exactly at ceiling", maxPlausiblePowerWatts, maxPlausiblePowerWatts},
		{"issue 94 spike low end", 20000, 0},
		{"issue 94 spike high end", 60000, 0},
		{"negative", -1, 0},
		{"NaN", math.NaN(), 0},
		{"positive infinity", math.Inf(1), 0},
		{"negative infinity", math.Inf(-1), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := plausiblePowerW(tt.in); got != tt.want {
				t.Errorf("plausiblePowerW(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeSocMetricsPowerRejectsImplausibleReadings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      SocMetrics
		wantPkg float64
		wantSys float64
	}{
		{
			name:    "kW spike on the SMC system key",
			in:      SocMetrics{CPUPower: 40, DRAMPower: 2, SystemPower: 24000, TotalPower: 42},
			wantPkg: 42,
			wantSys: 0,
		},
		{
			name:    "kW spike on the CPU rail",
			in:      SocMetrics{CPUPower: 48000, DRAMPower: 2, SystemPower: 20, TotalPower: 48002},
			wantPkg: 20,
			wantSys: 20,
		},
		{
			name:    "kW spike on the DRAM rail",
			in:      SocMetrics{CPUPower: 40, DRAMPower: 30000, SystemPower: 20, TotalPower: 30040},
			wantPkg: 20,
			wantSys: 20,
		},
		{
			name:    "every rail garbage",
			in:      SocMetrics{CPUPower: 20000, GPUPower: 30000, ANEPower: 40000, DRAMPower: 50000, GPUSRAMPower: 60000, SystemPower: 70000, TotalPower: 250000},
			wantPkg: 0,
			wantSys: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeSocMetricsPower(tt.in)
			if math.Abs(got.TotalPower-tt.wantPkg) > 1e-6 {
				t.Errorf("TotalPower = %.3f, want %.3f", got.TotalPower, tt.wantPkg)
			}
			if math.Abs(got.SystemPower-tt.wantSys) > 1e-6 {
				t.Errorf("SystemPower = %.3f, want %.3f", got.SystemPower, tt.wantSys)
			}
			if got.TotalPower > maxPlausiblePowerWatts {
				t.Errorf("TotalPower %.3f still exceeds the plausibility ceiling", got.TotalPower)
			}
			for _, rail := range []struct {
				name string
				v    float64
			}{
				{"cpu", got.CPUPower}, {"gpu", got.GPUPower}, {"ane", got.ANEPower},
				{"dram", got.DRAMPower}, {"gpu_sram", got.GPUSRAMPower},
			} {
				if rail.v > maxPlausiblePowerWatts {
					t.Errorf("%s rail = %.3f, still implausible", rail.name, rail.v)
				}
			}
		})
	}
}

func TestNormalizeSocMetricsPowerKeepsPlausibleReadings(t *testing.T) {
	t.Parallel()

	in := SocMetrics{
		CPUPower:     38.2,
		GPUPower:     0.1,
		DRAMPower:    1.8,
		GPUSRAMPower: 3.5,
		SystemPower:  24.4,
		TotalPower:   43.6,
	}
	got := normalizeSocMetricsPower(in)
	if got.CPUPower != in.CPUPower || got.GPUPower != in.GPUPower ||
		got.DRAMPower != in.DRAMPower || got.GPUSRAMPower != in.GPUSRAMPower {
		t.Errorf("plausible rails were altered: %+v", got)
	}
	if math.Abs(got.TotalPower-43.6) > 1e-9 {
		t.Errorf("TotalPower = %.3f, want 43.6", got.TotalPower)
	}
	if math.Abs(got.SystemPower-(43.6-43.6)) > 1e-9 {
		t.Errorf("SystemPower = %.3f, want 0 (components already cover the total)", got.SystemPower)
	}
}

func TestNextPowerScaleRejectsIssue94Spike(t *testing.T) {
	t.Parallel()

	scale := 70.0
	for range 200 {
		scale = nextPowerScale(scale, plausiblePowerW(24500))
	}
	if scale > 100 {
		t.Fatalf("scale ran away to %.2f W on a repeated 24.5 kW sample", scale)
	}
}

func TestNextPowerScaleDecaysAfterTransient(t *testing.T) {
	t.Parallel()

	scale := nextPowerScale(60, 120)
	if scale <= 120 {
		t.Fatalf("scale = %.2f, want it to rise to at least the 120 W reading", scale)
	}
	peak := scale

	settled := scale
	for range 2000 {
		settled = nextPowerScale(settled, 30)
	}
	if settled >= peak {
		t.Fatalf("scale = %.2f after 2000 idle samples, want it to decay below the %.2f peak", settled, peak)
	}
	if math.Abs(settled-30*powerScaleHeadroom) > 0.01 {
		t.Errorf("scale settled at %.3f, want it to converge to 30 W + headroom (%.3f)", settled, 30*powerScaleHeadroom)
	}
}

func TestNextPowerScaleGrowsImmediatelyAndNeverDipsBelowHeadroom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		prev        float64
		watts       float64
		wantAtLeast float64
		wantAtMost  float64
	}{
		{"new high jumps straight up", 60, 300, 300, 300 * powerScaleHeadroom},
		{"steady state holds", 330, 300, 300 * powerScaleHeadroom, 330},
		{"idle floor", 0.1, 0, minPowerScale, minPowerScale},
		{"rises from the floor", 0.1, 50, 50 * powerScaleHeadroom, 50 * powerScaleHeadroom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nextPowerScale(tt.prev, tt.watts)
			if got < tt.wantAtLeast-1e-9 || got > tt.wantAtMost+1e-9 {
				t.Errorf("nextPowerScale(%v, %v) = %v, want in [%v, %v]",
					tt.prev, tt.watts, got, tt.wantAtLeast, tt.wantAtMost)
			}
		})
	}
}
