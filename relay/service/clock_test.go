package service

import (
	"testing"
	"time"
)

func TestClockPlausible(t *testing.T) {
	// A Pi with no RTC and no network boots to 1970; anything at or after
	// the cutoff means the clock has been set.
	if !ClockPlausible() {
		t.Errorf("clock should be plausible: now=%s cutoff=%s", time.Now(), minValidTime)
	}
	if minValidTime.After(time.Now()) {
		t.Error("minValidTime is in the future — the cutoff would reject every real reading")
	}
}
