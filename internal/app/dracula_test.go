package app

import (
	"testing"

	ui "github.com/metaspartan/gotui/v5"
)

func TestIsDraculaTheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Dracula", "dracula", true},
		{"Mocha is Catppuccin", "mocha", false},
		{"Frappe is Catppuccin", "frappe", false},
		{"Named color", "green", false},
		{"Hex color", "#bd93f9", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsDraculaTheme(tt.input); got != tt.want {
				t.Errorf("IsDraculaTheme(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetDraculaHex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Primary", "Primary", "#bd93f9"},
		{"Text", "Text", "#f8f8f2"},
		{"Base", "Base", "#282a36"},
		{"Unknown falls back to white", "Nope", "#ffffff"},
		{"Empty falls back to white", "", "#ffffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := GetDraculaHex(tt.input); got != tt.want {
				t.Errorf("GetDraculaHex(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetDraculaHexReturnsValidHex(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Primary", "Text", "Base"} {
		if got := GetDraculaHex(name); !IsHexColor(got) {
			t.Errorf("GetDraculaHex(%q) = %q, which is not a valid hex color", name, got)
		}
	}
}

func TestGetDraculaPalette(t *testing.T) {
	t.Parallel()

	if got := GetDraculaPalette("dracula"); got == nil {
		t.Fatal(`GetDraculaPalette("dracula") = nil, want palette`)
	}

	for _, name := range []string{"mocha", "green", ""} {
		if got := GetDraculaPalette(name); got != nil {
			t.Errorf("GetDraculaPalette(%q) = %v, want nil", name, got)
		}
	}
}

func TestDraculaPaletteMatchesHexMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hex  string
		want ui.Color
	}{
		{"Primary is Purple", GetDraculaHex("Primary"), Dracula.Purple},
		{"Text is Foreground", GetDraculaHex("Text"), Dracula.Foreground},
		{"Base is Background", GetDraculaHex("Base"), Dracula.Background},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := ParseHexColor(tt.hex)
			if err != nil {
				t.Fatalf("ParseHexColor(%q) returned error: %v", tt.hex, err)
			}
			if parsed != tt.want {
				t.Errorf("ParseHexColor(%q) = %v, want %v", tt.hex, parsed, tt.want)
			}
		})
	}
}

func TestDraculaRegisteredInThemeOrder(t *testing.T) {
	t.Parallel()

	found := false
	for _, name := range themeOrder {
		if name == "dracula" {
			found = true
			break
		}
	}
	if !found {
		t.Error("dracula is missing from themeOrder, so the c key will never cycle to it")
	}

	if _, ok := colorMap["dracula"]; !ok {
		t.Error("dracula is missing from colorMap")
	}

	if _, ok := bgColorMap["dracula-base"]; !ok {
		t.Error("dracula-base is missing from bgColorMap")
	}
}

func TestIsPaletteTheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Dracula", "dracula", true},
		{"Mocha", "mocha", true},
		{"Macchiato", "macchiato", true},
		{"Frappe", "frappe", true},
		{"Named color", "green", false},
		{"1977", "1977", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPaletteTheme(tt.input); got != tt.want {
				t.Errorf("IsPaletteTheme(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetPaletteHexRoutesByTheme(t *testing.T) {
	t.Parallel()

	if got, want := GetPaletteHex("dracula", "Primary"), "#bd93f9"; got != want {
		t.Errorf("GetPaletteHex(dracula, Primary) = %q, want %q", got, want)
	}
	if got, want := GetPaletteHex("mocha", "Primary"), GetCatppuccinHex("mocha", "Primary"); got != want {
		t.Errorf("GetPaletteHex(mocha, Primary) = %q, want %q", got, want)
	}
}
