package main

import (
	"bytes"
	"net"
	"testing"
)

func TestBuildMagicPacket(t *testing.T) {
	macStr := "00:11:22:33:44:55"
	packet, err := buildMagicPacket(macStr)
	if err != nil {
		t.Fatalf("buildMagicPacket failed: %v", err)
	}

	if len(packet) != 102 {
		t.Fatalf("expected packet length 102, got %d", len(packet))
	}

	// First 6 bytes must be 0xFF
	expectedSync := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(packet[:6], expectedSync) {
		t.Fatalf("expected sync bytes FF FF FF FF FF FF, got %v", packet[:6])
	}

	// Next 16 repetitions of 6 bytes MAC
	parsedMAC, _ := net.ParseMAC(macStr)
	for i := 0; i < 16; i++ {
		chunk := packet[6+i*6 : 6+(i+1)*6]
		if !bytes.Equal(chunk, parsedMAC) {
			t.Fatalf("repetition %d does not match MAC: %v vs %v", i, chunk, parsedMAC)
		}
	}
}

func TestBuildMagicPacketInvalid(t *testing.T) {
	invalidCases := []string{
		"",
		"invalid",
		"00:11:22:33:44",          // 5 bytes
		"00:11:22:33:44:55:66:77", // EUI-64 (8 bytes, not supported by standard WoL)
		"ZZ:ZZ:ZZ:ZZ:ZZ:ZZ",
	}

	for _, c := range invalidCases {
		if _, err := buildMagicPacket(c); err == nil {
			t.Fatalf("expected error for invalid MAC %q, got nil", c)
		}
	}
}

func TestGetPrimaryNetworkInfo(t *testing.T) {
	info := getPrimaryNetworkInfo()
	// Should not panic, and if an interface exists, MAC should parse
	if info.MACAddress != "" {
		if _, err := net.ParseMAC(info.MACAddress); err != nil {
			t.Fatalf("detected MAC is invalid: %s (err: %v)", info.MACAddress, err)
		}
	}
}
