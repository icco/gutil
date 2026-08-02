// Package hexdate renders a date as the hexadecimal count of days since a
// fixed epoch.
package hexdate

import (
	"fmt"
	"time"
)

// Root is the epoch days are counted from.
var Root = time.Date(1988, time.February, 22, 0, 0, 0, 0, time.UTC)

// Date is a whole number of days since Root.
type Date struct {
	Days int64
}

// Now returns the number of whole days elapsed since Root.
func Now() *Date {
	delta := time.Since(Root)

	return &Date{Days: int64(delta.Hours()) / 24}
}

// String renders the day count in uppercase hexadecimal.
func (d *Date) String() string {
	return fmt.Sprintf("%X", d.Days)
}
