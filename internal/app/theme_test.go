// Copyright (c) 2024-2026 Carsen Klock under MIT License
// theme_test.go - pins the per-component gauge color scheme across theme families.
package app

import (
	"testing"

	ui "github.com/metaspartan/gotui/v5"
	w "github.com/metaspartan/gotui/v5/widgets"
)

// withTestGauges installs real gauge widgets into the package globals for the
// duration of fn, so the theme helpers have something to write to. The gauge
// pointers are package-level state shared with the running UI, so they are
// saved and restored rather than left mutated.
//
// Callers must NOT use t.Parallel(): these helpers write the same package
// globals, so concurrent callers overwrite each other's gauges. The race
// detector catches it, but the visible symptom is a flavor reading another's
// palette (e.g. mocha's gauges picking up latte's blue).
func withTestGauges(t *testing.T, fn func(cpu, gpu, memory, ane, pressure *w.Gauge)) {
	t.Helper()
	savedCPU, savedGPU := cpuGauge, gpuGauge
	savedMem, savedANE := memoryGauge, aneGauge
	savedPressure := memoryPressureGauge
	t.Cleanup(func() {
		cpuGauge, gpuGauge = savedCPU, savedGPU
		memoryGauge, aneGauge = savedMem, savedANE
		memoryPressureGauge = savedPressure
	})
	cpuGauge, gpuGauge = w.NewGauge(), w.NewGauge()
	memoryGauge, aneGauge = w.NewGauge(), w.NewGauge()
	memoryPressureGauge = w.NewGauge()
	fn(cpuGauge, gpuGauge, memoryGauge, aneGauge, memoryPressureGauge)
}

// TestSingleColorThemeGaugesAreUniform locks the behavior that ordinary
// single-color presets (green, red, blue, coffee, ...) paint every gauge the
// same as the surrounding UI.
func TestSingleColorThemeGaugesAreUniform(t *testing.T) {
	preset := ui.ColorCoral
	withTestGauges(t, func(cpu, gpu, memory, ane, _ *w.Gauge) {
		applyThemeToGauges(preset)
		for _, g := range []*w.Gauge{cpu, gpu, memory, ane} {
			if g.BarColor != preset {
				t.Errorf("BarColor = %v, want %v for every gauge", g.BarColor, preset)
			}
			if g.BorderStyle.Fg != preset {
				t.Errorf("BorderStyle.Fg = %v, want %v", g.BorderStyle.Fg, preset)
			}
			if g.TitleStyle.Fg != preset {
				t.Errorf("TitleStyle.Fg = %v, want %v", g.TitleStyle.Fg, preset)
			}
		}
	})
}

// TestTheme1977GaugeColors pins the 1977 accent scheme. 1977 is the one theme
// with no single color: the author marked it "Special theme without a single
// color" in themeOrder and re-applies these gauges every tick
// (app.go updateCPUCoreWidget / updateGPUCoreWidget) for dynamic saturation.
// CPU stays green to match the theme's own text color; GPU, memory, and ANE
// are deliberate accents.
func TestTheme1977GaugeColors(t *testing.T) {
	want := []struct {
		name  string
		gauge *w.Gauge
		color ui.Color
	}{
		{"cpu", nil, ui.ColorGreen},
		{"gpu", nil, ui.ColorMagenta},
		{"memory", nil, ui.ColorBlue},
		{"ane", nil, ui.ColorRed},
	}
	withTestGauges(t, func(cpu, gpu, memory, ane, _ *w.Gauge) {
		want[0].gauge, want[1].gauge = cpu, gpu
		want[2].gauge, want[3].gauge = memory, ane
		update1977GaugeColors()
		for _, c := range want {
			if c.gauge.BarColor != c.color {
				t.Errorf("1977 %s gauge = %v, want %v", c.name, c.gauge.BarColor, c.color)
			}
		}
	})
}

// TestCatppuccinGaugeSemanticColors pins the semantic scheme shared by every
// Catppuccin flavor: CPU green, GPU blue, memory yellow, ANE lavender. These
// are NOT the flavor's primary color, and that is intentional — the gauges
// answer "which component am I looking at", while the block/paragraph borders
// answer "which theme is this". See applyCatppuccinFullTheme, which uses
// primaryColor for the chrome and applyCatppuccinThemeToGauges for the gauges.
// The same split exists in the Dracula theme, so this is a family convention
// rather than a Catppuccin quirk.
func TestCatppuccinGaugeSemanticColors(t *testing.T) {
	flavors := map[string]*CatppuccinPalette{
		"frappe":    &CatppuccinFrappe,
		"macchiato": &CatppuccinMacchiato,
		"mocha":     &CatppuccinMocha,
		"latte":     &CatppuccinLatte,
	}
	for name, p := range flavors {
		t.Run(name, func(t *testing.T) {
			withTestGauges(t, func(cpu, gpu, memory, ane, _ *w.Gauge) {
				applyCatppuccinThemeToGauges(p)
				for _, c := range []struct {
					name  string
					gauge *w.Gauge
					color ui.Color
				}{
					{"cpu", cpu, p.Green},
					{"gpu", gpu, p.Blue},
					{"memory", memory, p.Yellow},
					{"ane", ane, p.Lavender},
				} {
					if c.gauge.BarColor != c.color {
						t.Errorf("%s %s gauge = %v, want %v", name, c.name, c.gauge.BarColor, c.color)
					}
					if c.gauge.LabelStyle.Fg != p.Subtext0 {
						t.Errorf("%s %s label = %v, want Subtext0 %v", name, c.name, c.gauge.LabelStyle.Fg, p.Subtext0)
					}
				}
			})
		})
	}
}

// TestDraculaGaugeSemanticColors pins Dracula's variant of the same convention:
// CPU green, GPU cyan, memory yellow, ANE purple.
func TestDraculaGaugeSemanticColors(t *testing.T) {
	withTestGauges(t, func(cpu, gpu, memory, ane, _ *w.Gauge) {
		applyDraculaThemeToGauges(&Dracula)
		for _, c := range []struct {
			name  string
			gauge *w.Gauge
			color ui.Color
		}{
			{"cpu", cpu, Dracula.Green},
			{"gpu", gpu, Dracula.Cyan},
			{"memory", memory, Dracula.Yellow},
			{"ane", ane, Dracula.Purple},
		} {
			if c.gauge.BarColor != c.color {
				t.Errorf("dracula %s gauge = %v, want %v", c.name, c.gauge.BarColor, c.color)
			}
		}
	})
}

// TestPaletteThemeGaugesDifferFromPrimary is the regression guard for the
// question raised in issue #93: for frappe/macchiato/mocha the GPU and memory
// gauge colors are intentionally different from the theme's primary color.
// If this test ever needs deleting, the design changed on purpose.
func TestPaletteThemeGaugesDifferFromPrimary(t *testing.T) {
	primaries := map[string]ui.Color{
		"frappe":    CatppuccinFrappe.Mauve,
		"macchiato": CatppuccinMacchiato.Sapphire,
		"mocha":     CatppuccinMocha.Peach,
	}
	for name, primary := range primaries {
		t.Run(name, func(t *testing.T) {
			withTestGauges(t, func(_, gpu, memory, _, _ *w.Gauge) {
				applyCatppuccinThemeToGauges(GetCatppuccinPalette(name))
				if gpu.BarColor == primary {
					t.Errorf("%s gpu gauge == primary %v; semantic accents should differ", name, primary)
				}
				if memory.BarColor == primary {
					t.Errorf("%s memory gauge == primary %v; semantic accents should differ", name, primary)
				}
			})
		})
	}
}

// TestCustomThemeGaugeFallback verifies per-component custom colors win, and an
// unset component falls back to the theme foreground instead of staying on the
// previous theme's accent.
func TestCustomThemeGaugeFallback(t *testing.T) {
	fg := ui.ColorSkyBlue
	custom := &CustomThemeConfig{GPU: "#ff0000"}
	// Compare against the parsed truecolor, not ui.ColorRed: ParseHexColor
	// returns a 24-bit RGB color, while ui.ColorRed is the ANSI palette index 1.
	// They render the same red but are not equal values.
	wantGPU, err := ParseHexColor("#ff0000")
	if err != nil {
		t.Fatalf("ParseHexColor: %v", err)
	}
	withTestGauges(t, func(cpu, gpu, _, _, _ *w.Gauge) {
		applyCustomGaugeColors(custom, fg)
		if got := gpu.BarColor; got != wantGPU {
			t.Errorf("custom gpu gauge = %v, want %v", got, wantGPU)
		}
		if cpu.BarColor != fg {
			t.Errorf("unset cpu gauge = %v, want foreground fallback %v", cpu.BarColor, fg)
		}
	})
}
