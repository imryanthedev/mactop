// Copyright (c) 2024-2026 Carsen Klock under MIT License
// fan_cli.go - one-shot headless fan control (no TUI required)
//
//	--fan-status        print current fan state as JSON and exit
//	--fan-set <value>   pin fans to an RPM / percent / min / max (requires root)
//	--fan-set auto      alias for --fan-auto
//	--fan-auto          restore automatic fan control and exit (requires root)
//	--fan-id <n>        restrict --fan-set to a single fan ID
//
// Unlike the interactive --fan-control TUI, these commands intentionally leave
// the fans in manual mode after exit — that is what makes them usable from
// scripts, cron, and SSH. The trade-off is that nothing restores automatic
// control on exit, so every successful --fan-set prints a reminder to run
// `sudo mactop --fan-auto`.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/metaspartan/mactop/v2/internal/i18n"
)

var (
	errFanSpecInvalid = errors.New("invalid fan target value")
	errFanNoMaxRPM    = errors.New("fan reports no max RPM")
	errFanUnknownID   = errors.New("unknown fan ID")
)

// fanTarget pairs a fan with the resolved RPM that --fan-set will write to it.
type fanTarget struct {
	fan FanInfo
	rpm int
}

// fanVerifyToleranceRPM absorbs SMC fixed-point quantization (fpe2 stores
// quarter-RPM steps) when comparing a read-back target against what was written.
const fanVerifyToleranceRPM = 25

// fanCLIRequested reports whether any one-shot fan command flag was given.
func fanCLIRequested() bool {
	return fanStatusFlag || fanAutoFlag || fanSetSpec != ""
}

// runFanCLI executes the requested one-shot fan command and exits the process.
func runFanCLI() {
	os.Exit(fanCLIMain())
}

func fanCLIMain() int {
	// --fan-set auto reads naturally in scripts; treat it as --fan-auto.
	if strings.EqualFold(strings.TrimSpace(fanSetSpec), "auto") {
		fanSetSpec = ""
		fanAutoFlag = true
	}
	switch {
	case fanSetSpec != "":
		return runFanSet(fanSetSpec, fanSetID)
	case fanAutoFlag:
		return runFanAuto()
	default:
		return runFanStatus()
	}
}

func runFanStatus() int {
	fans := GetFanList()
	printFanJSON(fans)
	if len(fans) == 0 {
		fmt.Fprintln(os.Stderr, i18n.T("Fan_NoFansDetected"))
	}
	return 0
}

func runFanAuto() int {
	if !fanControlHasRoot() {
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_NeedsRoot")+"\n", "--fan-auto")
		return 1
	}
	if len(GetFanList()) == 0 {
		// Restoring auto on a fanless Mac is vacuously successful — keep the
		// command idempotent for scripted cleanup.
		printFanJSON(nil)
		fmt.Fprintln(os.Stderr, i18n.T("Fan_NoFansDetected"))
		return 0
	}
	if err := ResetFansToAuto(); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_WriteFailed")+"\n", err)
		return 1
	}
	printFanJSON(GetFanList())
	fmt.Fprintln(os.Stderr, i18n.T("FanCLI_AutoRestored"))
	return 0
}

func runFanSet(spec string, fanID int) int {
	// Reads and validation need no privileges — do them first so a bad value
	// or fan ID gets its specific error even without sudo.
	fans := GetFanList()
	if len(fans) == 0 {
		fmt.Fprintln(os.Stderr, i18n.T("Fan_NoFansDetected"))
		return 1
	}
	targets, err := resolveFanTargets(fans, spec, fanID)
	if err != nil {
		printFanResolveError(err, spec, fanID, len(fans))
		return 1
	}
	if !fanControlHasRoot() {
		cmd := "--fan-set " + spec
		if fanID >= 0 {
			cmd += fmt.Sprintf(" --fan-id %d", fanID)
		}
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_NeedsRoot")+"\n", cmd)
		return 1
	}

	// All targets validated — write. Force-test first (Intel-era key, absent
	// on Apple Silicon, best-effort), then per fan: manual mode + target RPM.
	_ = SetFanForceTest(true)
	var writeErr error
	rec := func(e error) {
		if e != nil && writeErr == nil {
			writeErr = e
		}
	}
	for _, t := range targets {
		rec(SetFanMode(t.fan.ID, 1))
		rec(SetFanTarget(t.fan.ID, t.rpm))
	}
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_WriteFailed")+"\n", writeErr)
		return 1
	}

	// Read back and verify: SMC writes can return success yet be rejected by
	// the OS (the same failure mode the TUI surfaces via fanControlWriteFailed).
	after := GetFanList()
	printFanJSON(after)
	for _, t := range targets {
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_FanPinned")+"\n", t.fan.ID, t.rpm)
	}
	if !verifyFanTargets(after, targets) {
		fmt.Fprintln(os.Stderr, i18n.T("FanCLI_VerifyFailed"))
		return 1
	}
	fmt.Fprintln(os.Stderr, i18n.T("FanCLI_ManualWarning"))
	return 0
}

// resolveFanTargets picks which fans --fan-set applies to (all fans, or the
// one selected with --fan-id) and computes each fan's clamped target RPM.
// Validation happens for every fan before any SMC write, so a bad value never
// leaves the fans half-configured.
func resolveFanTargets(fans []FanInfo, spec string, fanID int) ([]fanTarget, error) {
	selected := fans
	if fanID >= 0 {
		selected = nil
		for _, f := range fans {
			if f.ID == fanID {
				selected = append(selected, f)
			}
		}
		if len(selected) == 0 {
			return nil, errFanUnknownID
		}
	}
	targets := make([]fanTarget, 0, len(selected))
	for _, f := range selected {
		rpm, err := parseFanTargetSpec(spec, f)
		if err != nil {
			return nil, err
		}
		targets = append(targets, fanTarget{fan: f, rpm: rpm})
	}
	return targets, nil
}

// parseFanTargetSpec resolves a --fan-set value against one fan's RPM range.
// Accepted forms: "min", "max", a percentage like "60%" (mapped onto the fan's
// min..max range), or an absolute RPM. The result is clamped to the fan's
// reported range (mirroring the C layer) so read-back verification compares
// against what was actually written.
func parseFanTargetSpec(spec string, fan FanInfo) (int, error) {
	s := strings.ToLower(strings.TrimSpace(spec))
	switch s {
	case "min":
		return fan.MinRPM, nil
	case "max":
		if fan.MaxRPM <= 0 {
			return 0, errFanNoMaxRPM
		}
		return fan.MaxRPM, nil
	}
	if pctStr, ok := strings.CutSuffix(s, "%"); ok {
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil || pct < 0 || pct > 100 {
			return 0, errFanSpecInvalid
		}
		if fan.MaxRPM <= fan.MinRPM {
			return 0, errFanNoMaxRPM
		}
		return fan.MinRPM + int(math.Round(float64(fan.MaxRPM-fan.MinRPM)*pct/100.0)), nil
	}
	rpm, err := strconv.Atoi(s)
	if err != nil || rpm < 0 {
		return 0, errFanSpecInvalid
	}
	return clampFanRPM(rpm, fan), nil
}

func clampFanRPM(rpm int, fan FanInfo) int {
	if rpm < fan.MinRPM {
		rpm = fan.MinRPM
	}
	if fan.MaxRPM > 0 && rpm > fan.MaxRPM {
		rpm = fan.MaxRPM
	}
	return rpm
}

// verifyFanTargets checks read-back state: every written fan should now be in
// manual mode with (approximately) the requested target RPM.
func verifyFanTargets(after []FanInfo, targets []fanTarget) bool {
	byID := make(map[int]FanInfo, len(after))
	for _, f := range after {
		byID[f.ID] = f
	}
	for _, t := range targets {
		f, ok := byID[t.fan.ID]
		if !ok {
			return false
		}
		diff := f.TargetRPM - t.rpm
		if diff < 0 {
			diff = -diff
		}
		if f.Mode != 1 || diff > fanVerifyToleranceRPM {
			return false
		}
	}
	return true
}

func printFanResolveError(err error, spec string, fanID, fanCount int) {
	switch {
	case errors.Is(err, errFanUnknownID):
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_UnknownFan")+"\n", fanID, fanCount)
	case errors.Is(err, errFanNoMaxRPM):
		fmt.Fprintln(os.Stderr, i18n.T("FanCLI_NoMaxRPM"))
	default:
		fmt.Fprintf(os.Stderr, i18n.T("FanCLI_InvalidValue")+"\n", spec)
	}
}

// printFanJSON writes the fan list to stdout as JSON (the machine-readable
// half of the command; human messages go to stderr). --pretty is honored.
func printFanJSON(fans []FanInfo) {
	hf := buildHeadlessFans(fans)
	if hf == nil {
		hf = []HeadlessFan{}
	}
	var data []byte
	var err error
	if headlessPretty {
		data, err = json.MarshalIndent(hf, "", "  ")
	} else {
		data, err = json.Marshal(hf)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("Headless_ErrorFormattingOutput")+"\n", err)
		return
	}
	fmt.Println(string(data))
}
