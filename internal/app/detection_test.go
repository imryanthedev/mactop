package app

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestParseOSC11Response(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		resp    string
		want    bool
		wantErr bool
	}{
		{"white background", "\033]11;rgb:ffff/ffff/ffff\a", true, false},
		{"black background", "\033]11;rgb:0000/0000/0000\a", false, false},
		{"st terminated", "\033]11;rgb:ffff/ffff/ffff\033\\", true, false},
		{"no rgb payload", "\033]11;?\a", false, true},
		{"short payload", "\033]11;rgb:ffff\a", false, true},
		{"non hex", "\033]11;rgb:zzzz/ffff/ffff\a", false, true},
		{"empty", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseOSC11Response(tt.resp)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got light=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("light = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanHex(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"ffff", "ffff"},
		{"FFFF", "FFFF"},
		{"12ab", "12ab"},
		{"12abZZ", "12ab"},
		{"", ""},
		{"zz", ""},
	}
	for _, tt := range tests {
		if got := cleanHex(tt.in); got != tt.want {
			t.Errorf("cleanHex(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLightModeOverride(t *testing.T) {
	tests := []struct {
		env    string
		want   bool
		wantOK bool
	}{
		{"1", true, true},
		{"true", true, true},
		{"light", true, true},
		{"on", true, true},
		{"0", false, true},
		{"false", false, true},
		{"dark", false, true},
		{"", false, false},
		{"maybe", false, false},
	}

	for _, tt := range tests {
		t.Run("env="+tt.env, func(t *testing.T) {
			t.Setenv("MACTOP_LIGHT_MODE", tt.env)
			got, ok := lightModeOverride()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("light = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadOSCResponseTimesOutOnSilentFD(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	start := time.Now()
	if _, err := readOSCResponse(int(r.Fd()), 60*time.Millisecond); err == nil {
		t.Fatal("expected a timeout error on a silent fd")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("readOSCResponse blocked for %v, want a prompt timeout", elapsed)
	}
}

func TestReadOSCResponseConsumesExactlyTheResponse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	resp := "\033]11;rgb:ffff/ffff/ffff\a"
	if _, err := w.Write([]byte(resp)); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readOSCResponse(int(r.Fd()), time.Second)
	if err != nil {
		t.Fatalf("readOSCResponse: %v", err)
	}
	if got != resp {
		t.Errorf("read %q, want %q", got, resp)
	}
	if n := drainAvailable(int(r.Fd())); n != 0 {
		t.Errorf("read-ahead left %d extra byte(s) on the fd", n)
	}
}

func TestReadOSCResponseStopsAtTerminator(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if _, err := w.Write([]byte("\033]11;rgb:0000/0000/0000\033\\trailing")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readOSCResponse(int(r.Fd()), time.Second)
	if err != nil {
		t.Fatalf("readOSCResponse: %v", err)
	}
	if strings.Contains(got, "trailing") {
		t.Errorf("read past the ST terminator: %q", got)
	}
	if isLight, err := parseOSC11Response(got); err != nil || isLight {
		t.Errorf("parse = %v, %v; want a dark background with no error", isLight, err)
	}
}

func TestReadOSCResponseLeavesNoGoroutineBehind(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	before := runtime.NumGoroutine()
	for range 20 {
		if _, err := readOSCResponse(int(r.Fd()), 20*time.Millisecond); err == nil {
			t.Fatal("expected timeout")
		}
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutines grew from %d to %d across 20 timed-out reads", before, after)
	}
}

func drainAvailable(fd int) int {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 30)
	if err != nil || n == 0 {
		return 0
	}
	var buf [256]byte
	total := 0
	for {
		m, err := unix.Read(fd, buf[:])
		if m > 0 {
			total += m
		}
		if err != nil {
			return total
		}
		if m == 0 {
			return total
		}
	}
}
