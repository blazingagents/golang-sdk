package blazingagents

import (
	"encoding/json"
	"fmt"
	"time"
)

// Date is a calendar day, sent as YYYY-MM-DD.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// IsZero reports whether d is the zero Date. That value is not a calendar day;
// String would format it as 0000-00-00.
func (d Date) IsZero() bool { return d == Date{} }

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return err
	}
	*d = Date{t.Year(), t.Month(), t.Day()}
	return nil
}

// DateRange is an inclusive range of UTC days. Params embed *DateRange; nil sends neither bound.
// A zero From or To is unset. Set both or neither: one side alone is not a range.
type DateRange struct {
	From Date `json:"from,omitzero" query:"from"`
	To   Date `json:"to,omitzero" query:"to"`
}

func (r *DateRange) check() error {
	if r == nil || r.From.IsZero() == r.To.IsZero() {
		return nil
	}
	return &ArgumentError{Name: "params.DateRange", Reason: "from and to must both be set or both be left unset"}
}
