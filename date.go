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
type DateRange struct {
	From Date `json:"from" query:"from"`
	To   Date `json:"to" query:"to"`
}
