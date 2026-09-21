// Package netcore stellt die Mesh-Netzbasis bereit:
// Portwahl (1x UDP), später UPnP/STUN/Hole-Punching.
package netcore

import (
	"fmt"
	"net"
)

// FindFreeUDPPort versucht zuerst preferred, sonst einen freien Port
// aus 49152-65535. Es wird bewusst nur EIN UDP-Port belegt.
func FindFreeUDPPort(preferred int) (int, error) {
	if preferred > 0 {
		if p, err := tryPort(preferred); err == nil {
			return p, nil
		}
	}
	for p := 49152; p <= 65535; p++ {
		if got, err := tryPort(p); err == nil {
			return got, nil
		}
	}
	return 0, fmt.Errorf("kein freier UDP-Port gefunden")
}

func tryPort(port int) (int, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return 0, err
	}
	c, err := net.ListenUDP("udp", addr)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	return port, nil
}

// ListenUDP bindet den Mesh-Socket (wird vom Daemon gehalten).
func ListenUDP(port int) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", addr)
}
