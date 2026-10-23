//go:build darwin

package main

import "testing"

func TestDarwinBundledEnginePath(t *testing.T) {
	for _, app := range []string{"/Applications/RelaisDesk.app", "/Users/test/Downloads/RelaisDesk Technicien.app"} {
		got := darwinBundledEnginePath(app + "/Contents/MacOS/RelaisDesk")
		want := app + "/Contents/Helpers/RelaisDeskEngine.app/Contents/MacOS/RelaisDesk"
		if got != want {
			t.Fatalf("engine path = %q, want %q", got, want)
		}
	}
}
