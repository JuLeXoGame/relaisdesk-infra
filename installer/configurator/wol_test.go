package main

import (
	"bytes"
	"net"
	"testing"
)

func TestBuildMagicPacket(t *testing.T) {
	macStr := "00:11:22:33:44:55"
	packet, err := BuildMagicPacket(macStr)
	if err != nil {
		t.Fatalf("BuildMagicPacket failed: %v", err)
	}

	if len(packet) != 102 {
		t.Fatalf("expected packet length 102, got %d", len(packet))
	}

	// First 6 bytes must be 0xFF
	expectedSync := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(packet[:6], expectedSync) {
		t.Fatalf("expected sync bytes FF FF FF FF FF FF, got %v", packet[:6])
	}

	// 16 repetitions of target MAC
	hw, _ := net.ParseMAC(macStr)
	for i := 0; i < 16; i++ {
		chunk := packet[6+i*6 : 6+(i+1)*6]
		if !bytes.Equal(chunk, hw) {
			t.Fatalf("repetition %d mismatch: %v vs %v", i, chunk, hw)
		}
	}
}

func TestBuildMagicPacketFormats(t *testing.T) {
	formats := []string{
		"00:11:22:33:44:55",
		"00-11-22-33-44-55",
		"0011.2233.4455",
	}
	for _, f := range formats {
		packet, err := BuildMagicPacket(f)
		if err != nil {
			t.Fatalf("failed for format %q: %v", f, err)
		}
		if len(packet) != 102 {
			t.Fatalf("invalid packet length for %q: %d", f, len(packet))
		}
	}
}

func TestBuildMagicPacketErrors(t *testing.T) {
	badCases := []string{
		"",
		"invalid-mac",
		"00:11:22:33:44",          // 5 octets
		"00:11:22:33:44:55:66:77", // 8 octets
	}
	for _, c := range badCases {
		if _, err := BuildMagicPacket(c); err == nil {
			t.Fatalf("expected error for %q, got nil", c)
		}
	}
}
