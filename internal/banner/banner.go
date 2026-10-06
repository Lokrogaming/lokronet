// Package banner: LOKRONET ASCII-Art (OpenCode-Stil) als Single Source of Truth.
// Quelle: internal/banner/banner.txt. CLI (version/setup/Menü) und Installer
// (via `lokronet version`) nutzen alle Print – keine kopierten Banner-Strings.
package banner

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed banner.txt
var art string

// Print gibt das LOKRONET-Logo aus (keine Farbcodes: skript- und logfreundlich).
func Print() {
	fmt.Println(strings.TrimSpace(art))
}
