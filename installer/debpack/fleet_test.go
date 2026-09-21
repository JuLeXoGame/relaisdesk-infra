package main

import (
	"strings"
	"testing"
)

func TestViewerRemovalStopsFleetOnlyOnRemoval(t *testing.T) {
	s := viewerPreRemovalScript()
	if !strings.Contains(s, `if [ "$1" = remove ]; then`) || !strings.Contains(s, "viewer-agent --unenroll") {
		t.Fatal("missing explicit service removal")
	}
	if strings.Contains(s, "\r") || strings.Contains(s, "rm -rf") || strings.Contains(s, "upgrade ];") {
		t.Fatal("unsafe package script")
	}
}
