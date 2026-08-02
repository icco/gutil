// Package neralie implements neralie time, a decimal clock that divides the
// day into 100000 parts instead of 86400 seconds.
package neralie

import (
	"fmt"
	"time"
)

// Time is a neralie clock reading: 1000 beats of 1000 pulses per day.
type Time struct {
	Beat  int
	Pulse int
}

// Now returns the current neralie time in UTC.
func Now() *Time {
	return FromTime(time.Now())
}

// FromTime converts a wall-clock time to neralie time. The conversion is
// always against UTC, so a neralie reading has no timezone.
func FromTime(t time.Time) *Time {
	utc := t.UTC()
	secToday := utc.Hour()*60*60 + utc.Minute()*60 + utc.Second()
	root := int((float64(secToday) / 86400.0) * 1000000.0)
	return &Time{
		Beat:  root / 1000,
		Pulse: root % 1000,
	}
}

// String renders the reading as "beat:pulse", each zero-padded to 3 digits.
func (t *Time) String() string {
	return fmt.Sprintf("%03d:%03d", t.Beat, t.Pulse)
}
