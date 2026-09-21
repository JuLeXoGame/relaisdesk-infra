package main

import (
	"bytes"
	"io"
	"os"
)

// Only called with a separately verified, administrator-owned native engine.
// This keeps a rebuilt launcher containing an old engine from advertising ACL support.
func fleetPeerVersionInFile(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	marker := []byte("RelaisDesk peer-auth-v1")
	buffer := make([]byte, 32768)
	tail := []byte{}
	for {
		n, e := f.Read(buffer)
		part := append(tail, buffer[:n]...)
		if bytes.Contains(part, marker) {
			return 1
		}
		if e != nil {
			if e != io.EOF {
				return 0
			}
			return 0
		}
		keep := len(marker) - 1
		if len(part) < keep {
			keep = len(part)
		}
		tail = append([]byte(nil), part[len(part)-keep:]...)
	}
}
