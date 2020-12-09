package util

import "time"

// StopTimer stops and drains the timer - please make sure timer channel doesn't have others listening to it
func StopTimer(timer *time.Timer) {
	if !timer.Stop() {
		// The Timer document is inaccurate with a bad exmaple - timer.Stop returning false doesn't necessarily
		// mean there is anything to drain in the channel. Blindly draining can block.
		// e.g. Stop is called after event is already fired. In this case draining the channel will block.
		// We use a default clause here to prevent this.
		select {
		case <-timer.C:
		default:
		}
	}
}

// ResetTimer stops the timer, drain events, and then resets the timer with a new duration
func ResetTimer(timer *time.Timer, d time.Duration) {
	StopTimer(timer)
	timer.Reset(d)
}

// Min returns a smaller integer out of two
func Min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// Max returns a larger integer out of two
func Max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
