// Package netinfo: lokale + öffentliche IP-Adressen (Stdlib only).
// Von CLI (ips-Command) und TUI (Network-Tab) gemeinsam genutzt.
package netinfo

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// IfAddr ist ein Interface + eine seiner Adressen.
type IfAddr struct {
	Iface string
	IP    string
	V6    bool
}

// LocalAddrs listet aktive, nicht-loopback Adressen (link-local ausgenommen).
// Leere Liste statt Fehler, wenn nichts ermittelbar ist.
func LocalAddrs() []IfAddr {
	var out []IfAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, IfAddr{Iface: iface.Name, IP: ip.String(), V6: ip.To4() == nil})
		}
	}
	return out
}

// PublicIP fragt externe Dienste (5s-Timeout). "" bei Offline/Fehler.
func PublicIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	for _, url := range []string{"https://api.ipify.org", "https://ifconfig.me"} {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		if ip := strings.TrimSpace(string(data)); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}
