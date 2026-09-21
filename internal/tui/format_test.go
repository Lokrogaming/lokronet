package tui

import (
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	cases := map[uint64]string{
		0:          "0 B",
		999:        "999 B",
		1024:       "1.0 KB",
		1536:       "1.5 KB",
		1048576:    "1.0 MB",
		1073741824: "1.0 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanBps(t *testing.T) {
	if got := humanBps(51.9); got != "52 B/s" {
		t.Errorf("humanBps = %q", got)
	}
	if got := humanBps(2048); !strings.HasSuffix(got, "/s") {
		t.Errorf("humanBps = %q", got)
	}
}

func TestFpShort(t *testing.T) {
	if got := fpShort("abc"); got != "abc" {
		t.Errorf("fpShort short = %q", got)
	}
	if got := fpShort("0123456789abcdefXYZ"); got != "0123456789abcdef…" {
		t.Errorf("fpShort long = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hello", 4); got != "hel…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("truncate empty = %q", got)
	}
}

func TestFmtTime(t *testing.T) {
	if got := fmtTime(0); got != "-" {
		t.Errorf("fmtTime(0) = %q", got)
	}
	if got := fmtTime(1790000000); got == "-" || got == "" {
		t.Errorf("fmtTime(valid) = %q", got)
	}
}

func TestRttText(t *testing.T) {
	if got := rttText(-1); got != "-" {
		t.Errorf("rttText(-1) = %q", got)
	}
	if got := rttText(24); got != "24 ms" {
		t.Errorf("rttText(24) = %q", got)
	}
}
