package tui

import (
	"fmt"
	"strings"
	"time"
)

// Pure Format-Helfer (ohne TTY testbar, siehe format_test.go).

// humanBytes formatiert Byte-Zähler kompakt.
func humanBytes(b uint64) string {
	const u = 1024.0
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	v := float64(b)
	for _, unit := range []string{"KB", "MB", "GB"} {
		v /= u
		if v < u {
			return fmt.Sprintf("%.1f %s", v, unit)
		}
	}
	return fmt.Sprintf("%.1f TB", v)
}

// humanBps formatiert Bytes/s kompakt.
func humanBps(bps float64) string {
	if bps < 1024 {
		return fmt.Sprintf("%.0f B/s", bps)
	}
	return humanBytes(uint64(bps)) + "/s"
}

// fpShort kürzt Fingerprints (voller String bei <= 16 Zeichen).
func fpShort(fp string) string {
	if len(fp) > 16 {
		return fp[:16] + "…"
	}
	return fp
}

// truncate kürzt auf max Runen (ohne Panic bei kurzen Strings).
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// fmtTime formatiert Unix-Zeit kompakt, "-" bei 0.
func fmtTime(unix int64) string {
	if unix <= 0 {
		return "-"
	}
	return time.Unix(unix, 0).Format("02.01. 15:04")
}

// rttText rendert Latenz ("-" bei unbekannt).
func rttText(ms int64) string {
	if ms < 0 {
		return "-"
	}
	return fmt.Sprintf("%d ms", ms)
}

// levelColor färbt Log-Zeilen nach Schlüsselwort (INFO default).
func levelColor(line string) string {
	u := strings.ToUpper(line)
	switch {
	case strings.Contains(u, "ERROR") || strings.Contains(u, "FEHLER") || strings.Contains(u, "TIMEOUT"):
		return styleErr.Render(line)
	case strings.Contains(u, "WARN") || strings.Contains(u, "GEDROSSELT") || strings.Contains(u, "PENDING"):
		return styleWarn.Render(line)
	default:
		return styleMuted.Render(line)
	}
}
