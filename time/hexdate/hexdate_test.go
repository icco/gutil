package hexdate

import (
	"testing"
	"time"
)

func TestDateString(t *testing.T) {
	t.Parallel()
	cases := map[int64]string{
		0:     "0",
		10:    "A",
		15:    "F",
		16:    "10",
		255:   "FF",
		4096:  "1000",
		13880: "3638",
	}
	for days, want := range cases {
		d := &Date{Days: days}
		if got := d.String(); got != want {
			t.Errorf("Date{%d}.String() = %q, want %q", days, got, want)
		}
	}
}

// Negative days would mean a time before Root. Go's %X renders those with a
// leading minus rather than as two's complement, which is the sane reading.
func TestDateStringNegative(t *testing.T) {
	t.Parallel()
	if got := (&Date{Days: -16}).String(); got != "-10" {
		t.Errorf("Date{-16}.String() = %q, want -10", got)
	}
}

func TestRootIsTheEpoch(t *testing.T) {
	t.Parallel()
	want := time.Date(1988, time.February, 22, 0, 0, 0, 0, time.UTC)
	if !Root.Equal(want) {
		t.Errorf("Root = %v, want %v", Root, want)
	}
}

// Now counts whole days since Root, so it must be positive and must line up
// with the elapsed time computed independently.
func TestNow(t *testing.T) {
	t.Parallel()
	got := Now()
	if got.Days <= 0 {
		t.Fatalf("Now().Days = %d, want a positive count since 1988", got.Days)
	}

	want := int64(time.Since(Root).Hours()) / 24
	// A day boundary can pass between the two calls.
	if diff := got.Days - want; diff < -1 || diff > 1 {
		t.Errorf("Now().Days = %d, want ~%d", got.Days, want)
	}
}

func TestNowStringIsHex(t *testing.T) {
	t.Parallel()
	s := Now().String()
	if s == "" {
		t.Fatal("Now().String() is empty")
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') {
			t.Errorf("Now().String() = %q contains non-hex %q", s, r)
			break
		}
	}
}
