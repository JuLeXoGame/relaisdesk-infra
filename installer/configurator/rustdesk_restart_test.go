package main

import (
	"testing"
	"time"
)

func TestWaitForConditionFalse(t *testing.T) {
	calls := 0
	if !waitForConditionFalse(func() bool {
		calls++
		return calls < 3
	}, 5*time.Second, time.Millisecond) {
		t.Fatal("expected condition to turn false")
	}
	if calls != 3 {
		t.Fatalf("expected 3 polls, got %d", calls)
	}
	if waitForConditionFalse(func() bool { return true }, 20*time.Millisecond, time.Millisecond) {
		t.Fatal("expected timeout when condition stays true")
	}
}
