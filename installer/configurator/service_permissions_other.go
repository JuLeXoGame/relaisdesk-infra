//go:build !windows

package main

import "os"

func protectServiceDirectory(dir string) error { return os.Chmod(dir, 0700) }
