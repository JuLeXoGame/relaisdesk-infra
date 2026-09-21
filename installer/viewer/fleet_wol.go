package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
)

type FleetNetworkInfo struct {
	MACAddress      string
	LocalIP         string
	SubnetBroadcast string
}

// getPrimaryNetworkInfo inspects network interfaces to find the primary physical adapter's
// MAC address, local IPv4, and subnet broadcast address.
func getPrimaryNetworkInfo() FleetNetworkInfo {
	info := FleetNetworkInfo{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return info
	}

	// First pass: look for active, non-loopback, non-virtual IPv4 interfaces
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "virtual") || strings.Contains(nameLower, "vmware") ||
			strings.Contains(nameLower, "vbox") || strings.Contains(nameLower, "tap") ||
			strings.Contains(nameLower, "tun") || strings.Contains(nameLower, "docker") ||
			strings.Contains(nameLower, "wsl") {
			continue
		}
		hw := iface.HardwareAddr.String()
		if len(iface.HardwareAddr) != 6 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ipv4 := ipnet.IP.To4()
				if ipv4 != nil && !ipv4.IsLoopback() && !ipv4.IsLinkLocalUnicast() {
					info.MACAddress = strings.ToLower(hw)
					info.LocalIP = ipv4.String()
					if len(ipnet.Mask) == 4 {
						bcast := net.IPv4(
							ipv4[0]|^ipnet.Mask[0],
							ipv4[1]|^ipnet.Mask[1],
							ipv4[2]|^ipnet.Mask[2],
							ipv4[3]|^ipnet.Mask[3],
						)
						info.SubnetBroadcast = bcast.String()
					}
					return info
				}
			}
		}
	}

	// Second pass fallback: any active non-loopback interface with a 6-byte MAC
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 6 {
			info.MACAddress = strings.ToLower(iface.HardwareAddr.String())
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipv4.IsLoopback() {
						info.LocalIP = ipv4.String()
						if len(ipnet.Mask) == 4 {
							bcast := net.IPv4(
								ipv4[0]|^ipnet.Mask[0],
								ipv4[1]|^ipnet.Mask[1],
								ipv4[2]|^ipnet.Mask[2],
								ipv4[3]|^ipnet.Mask[3],
							)
							info.SubnetBroadcast = bcast.String()
						}
						return info
					}
				}
			}
			return info
		}
	}

	return info
}

// buildMagicPacket constructs a standard 102-byte Wake-on-LAN magic packet.
func buildMagicPacket(macStr string) ([]byte, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(macStr))
	if err != nil {
		return nil, fmt.Errorf("adresse MAC invalide (%s) : %w", macStr, err)
	}
	if len(hw) != 6 {
		return nil, errors.New("l'adresse MAC WoL doit comporter 6 octets (MAC-48)")
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

// sendWakeOnLAN sends a magic packet to broadcast addresses on ports 9 and 7.
func sendWakeOnLAN(macStr string, customBroadcast ...string) error {
	packet, err := buildMagicPacket(macStr)
	if err != nil {
		return err
	}

	destinations := []string{}
	if len(customBroadcast) > 0 && strings.TrimSpace(customBroadcast[0]) != "" {
		destinations = append(destinations, strings.TrimSpace(customBroadcast[0]))
	} else {
		destinations = append(destinations, "255.255.255.255")
		// Collect subnet broadcast addresses from active interfaces
		if ifaces, err := net.Interfaces(); err == nil {
			for _, iface := range ifaces {
				if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
					continue
				}
				if addrs, err := iface.Addrs(); err == nil {
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok {
							if ipv4 := ipnet.IP.To4(); ipv4 != nil && len(ipnet.Mask) == 4 {
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
		return fmt.Errorf("échec d'émission WoL : %w", lastErr)
	}

	log.Printf("Parc : paquet Wake-on-LAN diffusé pour %s (%d paquets émis)", macStr, sentCount)
	return nil
}
