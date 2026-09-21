package main

import (
	"testing"
	"time"
)

func TestFleetCalculateNextWaitJitter(t *testing.T) {
	minExpected := 38250 * time.Millisecond // 45 * 0.85
	maxExpected := 51750 * time.Millisecond // 45 * 1.15

	seenDifferent := false
	var firstDuration time.Duration

	for i := 0; i < 100; i++ {
		d := fleetCalculateNextWait(45, 0)
		if d < minExpected || d > maxExpected {
			t.Fatalf("iteration %d: duration %v outside [%v, %v]", i, d, minExpected, maxExpected)
		}
		if i == 0 {
			firstDuration = d
		} else if d != firstDuration {
			seenDifferent = true
		}
	}
	if !seenDifferent {
		t.Fatal("expected randomized jitter across iterations, but all 100 durations were identical")
	}

	d60 := fleetCalculateNextWait(60, 0)
	if d60 < 51*time.Second || d60 > 69*time.Second {
		t.Fatalf("expected 60s jittered within [51s, 69s], got %v", d60)
	}

	d0 := fleetCalculateNextWait(0, 0)
	if d0 < minExpected || d0 > maxExpected {
		t.Fatalf("expected default 45s jittered within [%v, %v], got %v", minExpected, maxExpected, d0)
	}
}

func TestFleetCalculateNextWaitExponentialBackoff(t *testing.T) {
	for i := 0; i < 20; i++ {
		d := fleetCalculateNextWait(45, 1)
		if d < 2*time.Second || d > 5*time.Second {
			t.Fatalf("error 1: duration %v outside [2s, 5s]", d)
		}
	}

	for i := 0; i < 20; i++ {
		d := fleetCalculateNextWait(45, 2)
		if d < 2*time.Second || d > 10*time.Second {
			t.Fatalf("error 2: duration %v outside [2s, 10s]", d)
		}
	}

	for i := 0; i < 20; i++ {
		d := fleetCalculateNextWait(45, 3)
		if d < 2*time.Second || d > 20*time.Second {
			t.Fatalf("error 3: duration %v outside [2s, 20s]", d)
		}
	}

	for i := 0; i < 20; i++ {
		d := fleetCalculateNextWait(45, 15)
		if d < 2*time.Second || d > 120*time.Second {
			t.Fatalf("error 15: duration %v outside [2s, 120s]", d)
		}
	}
}

func TestFleetCalculateUpdateStagger(t *testing.T) {
	for i := 0; i < 20; i++ {
		d := fleetCalculateUpdateStagger(30)
		if d < 27*time.Second || d > 33*time.Second {
			t.Fatalf("expected stagger 30s jittered within [27s, 33s], got %v", d)
		}
	}

	for i := 0; i < 20; i++ {
		d := fleetCalculateUpdateStagger(0)
		if d < 5*time.Second || d > 45*time.Second {
			t.Fatalf("expected default stagger within [5s, 45s], got %v", d)
		}
	}
}
