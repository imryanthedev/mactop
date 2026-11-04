package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testFan(id, minRPM, maxRPM int) FanInfo {
	return FanInfo{ID: id, Name: "Fan " + string(rune('0'+id)), MinRPM: minRPM, MaxRPM: maxRPM}
}

func TestParseFanTargetSpec(t *testing.T) {
	t.Parallel()

	fan := testFan(0, 1000, 4000)
	cases := []struct {
		name    string
		spec    string
		fan     FanInfo
		want    int
		wantErr error
	}{
		{"min", "min", fan, 1000, nil},
		{"max", "max", fan, 4000, nil},
		{"min uppercase", "MIN", fan, 1000, nil},
		{"max padded", " Max ", fan, 4000, nil},
		{"zero percent", "0%", fan, 1000, nil},
		{"hundred percent", "100%", fan, 4000, nil},
		{"sixty percent", "60%", fan, 2800, nil},
		{"fractional percent", "12.5%", fan, 1375, nil},
		{"absolute rpm", "3000", fan, 3000, nil},
		{"rpm clamped low", "50", fan, 1000, nil},
		{"rpm clamped high", "9999", fan, 4000, nil},
		{"empty", "", fan, 0, errFanSpecInvalid},
		{"garbage", "fast", fan, 0, errFanSpecInvalid},
		{"negative rpm", "-100", fan, 0, errFanSpecInvalid},
		{"percent above 100", "150%", fan, 0, errFanSpecInvalid},
		{"negative percent", "-5%", fan, 0, errFanSpecInvalid},
		{"percent garbage", "abc%", fan, 0, errFanSpecInvalid},
		{"max without max rpm", "max", testFan(0, 1000, 0), 0, errFanNoMaxRPM},
		{"percent without range", "50%", testFan(0, 1000, 1000), 0, errFanNoMaxRPM},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseFanTargetSpec(tc.spec, tc.fan)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("parseFanTargetSpec(%q) error = %v, want %v", tc.spec, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("parseFanTargetSpec(%q) = %d, want %d", tc.spec, got, tc.want)
			}
		})
	}
}

func TestClampFanRPM(t *testing.T) {
	t.Parallel()

	fan := testFan(0, 1200, 3600)
	if got := clampFanRPM(100, fan); got != 1200 {
		t.Errorf("clamp below min = %d, want 1200", got)
	}
	if got := clampFanRPM(5000, fan); got != 3600 {
		t.Errorf("clamp above max = %d, want 3600", got)
	}
	if got := clampFanRPM(2000, fan); got != 2000 {
		t.Errorf("clamp in range = %d, want 2000", got)
	}
	// A zero max (unreadable key) must not clamp everything down to min
	noMax := testFan(0, 1200, 0)
	if got := clampFanRPM(5000, noMax); got != 5000 {
		t.Errorf("clamp with no max = %d, want 5000", got)
	}
}

func TestResolveFanTargets(t *testing.T) {
	t.Parallel()

	fans := []FanInfo{testFan(0, 1000, 4000), testFan(1, 1100, 3500)}

	t.Run("all fans", func(t *testing.T) {
		t.Parallel()
		targets, err := resolveFanTargets(fans, "100%", -1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("got %d targets, want 2", len(targets))
		}
		// Percent resolves per fan against its own range
		if targets[0].rpm != 4000 || targets[1].rpm != 3500 {
			t.Fatalf("targets = %d/%d, want 4000/3500", targets[0].rpm, targets[1].rpm)
		}
	})

	t.Run("single fan by ID", func(t *testing.T) {
		t.Parallel()
		targets, err := resolveFanTargets(fans, "min", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0].fan.ID != 1 || targets[0].rpm != 1100 {
			t.Fatalf("targets = %+v, want fan 1 at 1100", targets)
		}
	})

	t.Run("unknown fan ID", func(t *testing.T) {
		t.Parallel()
		if _, err := resolveFanTargets(fans, "min", 7); !errors.Is(err, errFanUnknownID) {
			t.Fatalf("error = %v, want errFanUnknownID", err)
		}
	})

	t.Run("invalid spec propagates", func(t *testing.T) {
		t.Parallel()
		if _, err := resolveFanTargets(fans, "warp9", -1); !errors.Is(err, errFanSpecInvalid) {
			t.Fatalf("error = %v, want errFanSpecInvalid", err)
		}
	})
}

func TestVerifyFanTargets(t *testing.T) {
	t.Parallel()

	targets := []fanTarget{{fan: testFan(0, 1000, 4000), rpm: 3000}}
	manual := func(id, target int) FanInfo {
		f := testFan(id, 1000, 4000)
		f.Mode = 1
		f.TargetRPM = target
		return f
	}

	cases := []struct {
		name  string
		after []FanInfo
		want  bool
	}{
		{"exact match", []FanInfo{manual(0, 3000)}, true},
		{"within tolerance", []FanInfo{manual(0, 3000+fanVerifyToleranceRPM)}, true},
		{"beyond tolerance", []FanInfo{manual(0, 3000+fanVerifyToleranceRPM+1)}, false},
		{"still in auto mode", []FanInfo{testFan(0, 1000, 4000)}, false},
		{"fan missing from readback", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := verifyFanTargets(tc.after, targets); got != tc.want {
				t.Fatalf("verifyFanTargets = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFanStatusIntegration runs the built binary with --fan-status and checks
// the stdout contract: a JSON array of fans (possibly empty on fanless Macs),
// exit code 0, no root required.
func TestFanStatusIntegration(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping integration test in CI environment")
	}

	projectRoot := filepath.Join("..", "..")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	binary := "mactop_fan_test_binary"
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	buildCmd.Dir = projectRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, out)
	}
	defer os.Remove(filepath.Join(projectRoot, binary))

	cmd := exec.CommandContext(ctx, "./"+binary, "--fan-status")
	cmd.Dir = projectRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("--fan-status failed: %v\nstderr: %s", err, stderr.String())
	}

	var fans []HeadlessFan
	if err := json.Unmarshal(stdout.Bytes(), &fans); err != nil {
		t.Fatalf("Failed to parse --fan-status JSON: %v\nOutput: %s", err, stdout.String())
	}
	for _, f := range fans {
		if f.Mode != "auto" && f.Mode != "manual" {
			t.Errorf("fan %d has unexpected mode %q", f.ID, f.Mode)
		}
	}
}
