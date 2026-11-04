package app

import (
	"strings"
	"testing"
)

func TestPowerSupplyRated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   PowerSupply
		want bool
	}{
		{"rated laptop", PowerSupply{AdapterConnected: true, AdapterWatts: 60}, true},
		{"unrated desktop", PowerSupply{AdapterConnected: false, AdapterWatts: 0}, false},
		{"no battery at all", PowerSupply{}, false},
		{"connected but unknown rating", PowerSupply{AdapterConnected: true}, false},
		{"rating without a connected supply", PowerSupply{AdapterConnected: false, AdapterWatts: 96}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.Rated(); got != tt.want {
				t.Errorf("Rated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPowerSupplyIsSelfConsistent(t *testing.T) {
	supply := GetPowerSupply()
	t.Logf("power supply: %+v", supply)

	if supply.AdapterWatts < 0 {
		t.Errorf("AdapterWatts = %d, must never be negative", supply.AdapterWatts)
	}
	if supply.Rated() && !supply.AdapterConnected {
		t.Error("Rated() is true while AdapterConnected is false")
	}
	if supply.OnACPower && supply.Source != "" && !strings.EqualFold(supply.Source, "AC Power") {
		t.Errorf("OnACPower is true but Source is %q", supply.Source)
	}
	if !supply.Rated() && supply.AdapterWatts != 0 {
		t.Errorf("unrated supply reports %d W; an unknown rating must read as 0", supply.AdapterWatts)
	}
}

func TestGetPowerSupplyIsStable(t *testing.T) {
	first := GetPowerSupply()
	second := GetPowerSupply()

	if first.AdapterWatts != second.AdapterWatts {
		t.Errorf("adapter rating changed between calls: %d then %d", first.AdapterWatts, second.AdapterWatts)
	}
	if first.AdapterDescription != second.AdapterDescription {
		t.Errorf("adapter description changed between calls: %q then %q", first.AdapterDescription, second.AdapterDescription)
	}
}

func TestPowerSupplyLine(t *testing.T) {
	line := powerSupplyLine()
	t.Logf("power supply line: %q", line)

	if line == "" {
		return
	}
	if !strings.Contains(line, ":") {
		t.Errorf("line %q has no label separator", line)
	}
	label, value, found := strings.Cut(line, ": ")
	if !found {
		t.Fatalf("line %q is not in label/value form", line)
	}
	if label == "" || value == "" {
		t.Errorf("line %q has an empty label or value", line)
	}
	if len(line) > 0 && line[len(line)-1] == ' ' {
		t.Errorf("line %q has trailing whitespace", line)
	}
}

func TestAdapterCacheStartsUnknown(t *testing.T) {
	if adapterKnown {
		t.Log("adapter already cached from an earlier test")
	}
	_, _, known := cachedAdapter()
	t.Logf("adapterKnown = %v", known)
	if known {
		watts, desc, _ := cachedAdapter()
		if watts <= 0 && desc == "" {
			t.Error("adapter marked known with an empty reading")
		}
	}
}
