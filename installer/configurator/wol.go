package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// BuildMagicPacket constructs a standard 102-byte Wake-on-LAN magic packet.
// Format: 6 bytes of 0xFF followed by 16 repetitions of the target 48-bit MAC address.
func BuildMagicPacket(macStr string) ([]byte, error) {
	macStr = strings.TrimSpace(macStr)
	if macStr == "" {
		return nil, errors.New("adresse MAC manquante")
	}
	hw, err := net.ParseMAC(macStr)
	if err != nil {
		return nil, fmt.Errorf("format d'adresse MAC invalide (%s) : %w", macStr, err)
	}
	if len(hw) != 6 {
		return nil, errors.New("l'adresse MAC doit comporter 6 octets (MAC-48/EUI-48)")
	}

	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:6+(i+1)*6], hw)
	}
	return packet, nil
}

// SendWakeOnLAN broadcasts the Wake-on-LAN magic packet across available local interfaces
// and to global broadcast on UDP ports 9 and 7.
// Returns the count of successfully sent UDP packets, or an error if none could be sent.
func SendWakeOnLAN(macStr string, customBroadcast ...string) (int, error) {
	packet, err := BuildMagicPacket(macStr)
	if err != nil {
		return 0, err
	}

	destinations := []string{}
	if len(customBroadcast) > 0 && strings.TrimSpace(customBroadcast[0]) != "" {
		destinations = append(destinations, strings.TrimSpace(customBroadcast[0]))
	} else {
		destinations = append(destinations, "255.255.255.255")
		// Query local interfaces for active subnet broadcast addresses
		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
					continue
				}
				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok {
						ipv4 := ipnet.IP.To4()
						if ipv4 != nil && len(ipnet.Mask) == 4 {
							bcast := net.IPv4(
								ipv4[0]|^ipnet.Mask[0],
								ipv4[1]|^ipnet.Mask[1],
								ipv4[2]|^ipnet.Mask[2],
								ipv4[3]|^ipnet.Mask[3],
							)
							destinations = append(destinations, bcast.String())
						}
					}
				}
			}
		}
	}

	ports := []int{9, 7}
	var sentCount int
	var lastErr error

	for _, dest := range destinations {
		for _, port := range ports {
			addrStr := fmt.Sprintf("%s:%d", dest, port)
			udpAddr, err := net.ResolveUDPAddr("udp4", addrStr)
			if err != nil {
				lastErr = err
				continue
			}
			conn, err := net.DialUDP("udp4", nil, udpAddr)
			if err != nil {
				lastErr = err
				continue
			}
			_, err = conn.Write(packet)
			conn.Close()
			if err != nil {
				lastErr = err
			} else {
				sentCount++
			}
		}
	}

	if sentCount == 0 && lastErr != nil {
		return 0, fmt.Errorf("échec d'envoi du paquet Wake-on-LAN : %w", lastErr)
	}

	return sentCount, nil
}
