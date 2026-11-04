package app

import (
	"strconv"
	"strings"
	"testing"

	w "github.com/metaspartan/gotui/v5/widgets"
	"github.com/metaspartan/mactop/v2/internal/i18n"
)

func setupPowerChartWidgets(t *testing.T) {
	t.Helper()
	i18n.Init("en")
	sparkline = w.NewSparkline()
	sparklineGroup = w.NewSparklineGroup(sparkline)
	powerHistoryChart = nil
	powerValues = make([]float64, 100)
	powerUsageHistory = make([]float64, 100)
	maxPowerSeen = minPowerScale
	t.Cleanup(func() {
		sparkline, sparklineGroup = nil, nil
	})
}

func maxWattsInTitle(t *testing.T, title string) float64 {
	t.Helper()
	_, after, ok := strings.Cut(title, "Max: ")
	if !ok {
		t.Fatalf("title %q has no Max: field", title)
	}
	num, _, _ := strings.Cut(after, " ")
	v, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil {
		t.Fatalf("title %q: Max value %q is not a number: %v", title, num, err)
	}
	return v
}

func TestTotalPowerChartSurvivesIssue94Spike(t *testing.T) {
	setupPowerChartWidgets(t)

	for range 50 {
		updateTotalPowerChart(64)
	}
	baseline := maxWattsInTitle(t, sparklineGroup.Title)

	for _, spike := range []float64{20000, 45000, 60000} {
		updateTotalPowerChart(spike)
	}

	shown := maxWattsInTitle(t, sparklineGroup.Title)
	if shown > maxPlausiblePowerWatts {
		t.Errorf("after a 20-60 kW spike the chart reports Max %.2f W (was %.2f W)", shown, baseline)
	}
	if strings.Contains(sparklineGroup.Title, "0000.00 W Total") {
		t.Errorf("chart shows the spike verbatim: %q", sparklineGroup.Title)
	}
	if got := powerUsageHistory[len(powerUsageHistory)-1]; got > maxPlausiblePowerWatts {
		t.Errorf("history buffer took the spike: %.2f W", got)
	}
	for i, v := range powerValues {
		if v > 8 {
			t.Errorf("sparkline point %d out of range: %.2f (max 8)", i, v)
		}
	}
}

func TestTotalPowerChartRecoversAfterTransient(t *testing.T) {
	setupPowerChartWidgets(t)

	for range 50 {
		updateTotalPowerChart(300)
	}
	peak := maxWattsInTitle(t, sparklineGroup.Title)
	if peak < 300 {
		t.Fatalf("scale = %.2f W, want it to have tracked the 300 W burst", peak)
	}

	for range 2000 {
		updateTotalPowerChart(30)
	}
	settled := maxWattsInTitle(t, sparklineGroup.Title)
	if settled >= peak {
		t.Fatalf("scale = %.2f W after the load dropped, want it below the %.2f W peak", settled, peak)
	}
	if settled > 30*powerScaleHeadroom+0.01 {
		t.Errorf("scale = %.2f W, want it to settle near 30 W + headroom", settled)
	}
}

func TestTotalPowerChartSteadyStateIsUnchanged(t *testing.T) {
	setupPowerChartWidgets(t)

	for range 50 {
		updateTotalPowerChart(64)
	}
	first := maxWattsInTitle(t, sparklineGroup.Title)
	for range 500 {
		updateTotalPowerChart(64)
	}
	steady := maxWattsInTitle(t, sparklineGroup.Title)

	if steady != 64*powerScaleHeadroom {
		t.Errorf("steady-state scale = %.4f W, want %.4f W", steady, 64*powerScaleHeadroom)
	}
	if steady > first+1e-9 {
		t.Errorf("scale drifted upward on a flat load: %.4f -> %.4f", first, steady)
	}
	flat := 64.0
	want := float64(int((flat / (flat * powerScaleHeadroom)) * 8))
	if got := powerValues[len(powerValues)-1]; got != want {
		t.Errorf("flat load maps to %.2f, want the unchanged %.2f of 8", got, want)
	}
}
