package main

import (
	"testing"
	"time"
)

func TestFleetBackoffSurvivesHugeErrorCounts(t *testing.T) {
	for _, errors := range []int{64, 100, 1000} {
		fullBackoffSeen := false
		for i := 0; i < 5; i++ {
			d := fleetCalculateNextWait(45, errors)
			if d < 2*time.Second || d > 120*time.Second {
				t.Fatalf("errors=%d: duration %v outside [2s, 120s]", errors, d)
			}
			if d != 2*time.Second {
				fullBackoffSeen = true
			}
		}
		if !fullBackoffSeen {
			t.Fatalf("errors=%d: backoff collapsed to the 2s minimum on all iterations", errors)
		}
	}
}

func TestQuoteWindowsCmdArg(t *testing.T) {
	cases := []struct{ in, want string }{
		{"--enroll", "--enroll"},
		{"PERM-ABCD-1234", "PERM-ABCD-1234"},
		{"", `""`},
		{`C:\Program Files\app.exe`, `"C:\Program Files\app.exe"`},
		{`p@ss word`, `"p@ss word"`},
		{`a"b`, `"a\"b"`},
		{`C:\my dir\`, `"C:\my dir\\"`},
		{`trailing\`, `trailing\`},
	}
	for _, tc := range cases {
		if got := quoteWindowsCmdArg(tc.in); got != tc.want {
			t.Errorf("quoteWindowsCmdArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
