package app

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const (
	lightModeQueryTimeout = 150 * time.Millisecond
	lightModeMaxResponse  = 128
)

func detectLightMode() bool {
	if v, ok := lightModeOverride(); ok {
		return v
	}
	if isLight, err := checkTerminalColorOSC11(); err == nil {
		return isLight
	}
	if isLight, err := checkCOLORFGBG(); err == nil {
		return isLight
	}
	if isLight, err := checkSystemTheme(); err == nil {
		return isLight
	}
	return false
}

func lightModeOverride() (bool, bool) {
	for _, name := range []string{"MACTOP_LIGHT_MODE", "MACTOP_FORCE_LIGHT"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
		case "1", "true", "yes", "on", "light":
			return true, true
		case "0", "false", "no", "off", "dark":
			return false, true
		}
	}
	return false, false
}

func checkTerminalColorOSC11() (bool, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return false, fmt.Errorf("stdin is not a terminal")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, err
	}
	defer term.Restore(fd, oldState)

	if _, err := os.Stdout.WriteString("\033]11;?\007"); err != nil {
		return false, err
	}

	resp, err := readOSCResponse(fd, lightModeQueryTimeout)
	if err != nil {
		return false, err
	}
	return parseOSC11Response(resp)
}

func readOSCResponse(fd int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var response []byte
	for len(response) < lightModeMaxResponse {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", fmt.Errorf("timeout waiting for OSC 11 response")
		}
		if err := waitReadable(fd, remaining); err != nil {
			return "", err
		}
		var buf [1]byte
		n, err := unix.Read(fd, buf[:])
		if err != nil {
			if err == unix.EINTR || err == unix.EAGAIN {
				continue
			}
			return "", err
		}
		if n == 0 {
			return "", fmt.Errorf("stdin closed during OSC 11 query")
		}
		response = append(response, buf[0])
		if buf[0] == 0x07 {
			break
		}
		if len(response) >= 2 && response[len(response)-2] == 0x1b && response[len(response)-1] == 0x5c {
			break
		}
	}
	return string(response), nil
}

func waitReadable(fd int, timeout time.Duration) error {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, int(timeout.Milliseconds())+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("timeout waiting for OSC 11 response")
		}
		return nil
	}
}

func parseOSC11Response(resp string) (bool, error) {
	_, colorStr, ok := strings.Cut(resp, "rgb:")
	if !ok {
		return false, fmt.Errorf("invalid response format")
	}

	parts := strings.Split(colorStr, "/")
	if len(parts) < 3 {
		return false, fmt.Errorf("invalid color format")
	}

	r, err1 := strconv.ParseUint(cleanHex(parts[0]), 16, 16)
	g, err2 := strconv.ParseUint(cleanHex(parts[1]), 16, 16)
	b, err3 := strconv.ParseUint(cleanHex(parts[2]), 16, 16)

	if err1 != nil || err2 != nil || err3 != nil {
		return false, fmt.Errorf("error parsing hex")
	}

	luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luminance > 65535.0*0.5 {
		return true, nil
	}
	return false, nil
}

func cleanHex(s string) string {
	var sb strings.Builder
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			sb.WriteRune(c)
		} else {
			break
		}
	}
	return sb.String()
}

func checkCOLORFGBG() (bool, error) {
	colorFGBG := os.Getenv("COLORFGBG")
	if colorFGBG == "" {
		return false, fmt.Errorf("COLORFGBG not set")
	}

	parts := strings.Split(colorFGBG, ";")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid COLORFGBG format")
	}

	bg, err := strconv.Atoi(parts[1])
	if err != nil {
		return false, err
	}

	if bg == 7 || bg == 15 || bg == 11 || bg == 14 || bg == 231 || bg == 255 {
		return true, nil
	}
	return false, nil
}

func checkSystemTheme() (bool, error) {
	cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
	out, err := cmd.Output()
	if err != nil {
		return true, nil
	}
	return strings.TrimSpace(string(out)) != "Dark", nil
}
