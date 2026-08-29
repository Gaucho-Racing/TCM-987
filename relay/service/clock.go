package service

import "time"

// minValidTime mirrors Mapache's minValidProducedAt. A Pi with no RTC and
// no internet boots to 1970 — anything before this date is pre-clock
// garbage. Keep the two cutoffs in lockstep.
var minValidTime = time.Date(2003, 10, 31, 0, 0, 0, 0, time.UTC)

const clockCheckInterval = 30 * time.Second

// ClockPlausible reports whether the local clock is at or after minValidTime.
func ClockPlausible() bool {
	return !time.Now().Before(minValidTime)
}
