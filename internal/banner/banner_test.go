package banner

import (
	"strings"
	"testing"
)

func TestArtHasThreeLines(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(art), "\n")
	if len(lines) != 4 {
		t.Fatalf("Banner braucht 4 Zeilen, hat %d", len(lines))
	}
	for _, l := range lines {
		if !strings.Contains(l, "█") {
			t.Fatalf("Zeile ohne Bloecke: %q", l)
		}
	}
}
