package app

import (
	"regexp"
	"strings"
	"testing"
)

var gotuiTag = regexp.MustCompile(`\]\([^)]*\)`)

func plainFanLine(rendered string) string {
	return strings.NewReplacer("[", "", "]", "").Replace(gotuiTag.ReplaceAllString(rendered, ""))
}

func m2UltraFan() FanInfo {
	return FanInfo{
		ID: 0, Name: "Fan 0",
		ActualRPM: 0, MinRPM: 1000, MaxRPM: 3500, TargetRPM: 0, Mode: 0,
		TachReadable: false,
	}
}

func healthyFan() FanInfo {
	return FanInfo{
		ID: 0, Name: "Fan 0",
		ActualRPM: 3454, MinRPM: 1350, MaxRPM: 5349, TargetRPM: 3455, Mode: 0,
		TachReadable: true,
	}
}

func TestFanRPMBarShowsNotAvailableWhenTachUnreadable(t *testing.T) {
	lines := fanRPMBar(m2UltraFan(), "white")
	if len(lines) == 0 {
		t.Fatal("fanRPMBar returned no lines")
	}
	head := lines[0]

	plain := plainFanLine(head)
	actual, rest, found := strings.Cut(plain, " / ")
	if !found {
		t.Fatalf("fan line is not in the expected shape: %q", plain)
	}
	if !strings.HasSuffix(strings.TrimSpace(actual), "N/A") {
		t.Errorf("RPM slot shows %q, want N/A (a 0 here claims stopped fans): %q",
			actual[strings.LastIndex(actual, " ")+1:], plain)
	}
	if !strings.HasPrefix(strings.TrimSpace(rest), "3500 RPM") {
		t.Errorf("max RPM was dropped, so the range is no longer meaningful: %q", plain)
	}
}

func TestFanRPMBarUnchangedWhenTachReadable(t *testing.T) {
	lines := fanRPMBar(healthyFan(), "white")
	if len(lines) == 0 {
		t.Fatal("fanRPMBar returned no lines")
	}
	head := lines[0]
	if !strings.Contains(head, "3454") {
		t.Errorf("healthy fan no longer shows its RPM: %q", head)
	}
	if strings.Contains(head, "N/A") {
		t.Errorf("healthy fan rendered as unavailable: %q", head)
	}
}

func TestBuildHeadlessFansCarriesTachReadable(t *testing.T) {
	t.Parallel()

	got := buildHeadlessFans([]FanInfo{m2UltraFan(), healthyFan()})
	if len(got) != 2 {
		t.Fatalf("got %d fans, want 2", len(got))
	}
	if got[0].TachReadable {
		t.Error("M2 Ultra signature reported as tach_readable")
	}
	if got[0].RPM != 0 {
		t.Errorf("RPM = %d, want the raw 0 preserved (tach_readable is the signal)", got[0].RPM)
	}
	if !got[1].TachReadable {
		t.Error("healthy fan reported as tach_readable=false")
	}
	if got[1].RPM != 3454 {
		t.Errorf("healthy RPM = %d, want 3454", got[1].RPM)
	}
}

func TestLiveFansHaveReadableTach(t *testing.T) {
	fans := GetFanList()
	if len(fans) == 0 {
		t.Skip("no fans on this host")
	}
	for _, f := range fans {
		t.Logf("%s: rpm=%d min=%d max=%d tach_readable=%v", f.Name, f.ActualRPM, f.MinRPM, f.MaxRPM, f.TachReadable)
		if f.MinRPM > 0 && f.ActualRPM == 0 && f.TachReadable {
			t.Errorf("%s: actual 0 with a non-zero floor was accepted as a measurement", f.Name)
		}
		if f.TachReadable && f.ActualRPM > 0 {
			continue
		}
		if f.TachReadable {
			t.Errorf("%s: marked readable but reported no RPM", f.Name)
		}
	}
}
