package main

import "time"

// waitForConditionFalse polls cond until it reports false or timeout elapses.
// It reports whether cond turned false in time.
func waitForConditionFalse(cond func() bool, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}
