package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const dateLayout = "2006-01-02"

// Date is a calendar date with no time of day, such as a join date or a last
// working day. It is stored in MongoDB as a UTC-midnight datetime and written
// in JSON as "YYYY-MM-DD".
type Date struct{ time.Time }

func NewDate(y int, m time.Month, d int) Date {
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("must be a date in YYYY-MM-DD format")
	}
	return Date{t}, nil
}

// Today returns the current calendar date in the given location.
func Today(loc *time.Location) Date {
	y, m, d := time.Now().In(loc).Date()
	return NewDate(y, m, d)
}

func (d Date) String() string      { return d.Time.Format(dateLayout) }
func (d Date) AddDays(n int) Date  { return Date{d.Time.AddDate(0, 0, n)} }
func (d Date) AddYears(n int) Date { return Date{d.Time.AddDate(n, 0, 0)} }
func (d Date) Before(o Date) bool  { return d.Time.Before(o.Time) }
func (d Date) After(o Date) bool   { return d.Time.After(o.Time) }
func (d Date) Equal(o Date) bool   { return d.Time.Equal(o.Time) }

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d Date) MarshalBSONValue() (byte, []byte, error) {
	t, data, err := bson.MarshalValue(d.Time)
	return byte(t), data, err
}

func (d *Date) UnmarshalBSONValue(t byte, data []byte) error {
	var tm time.Time
	if err := bson.UnmarshalValue(bson.Type(t), data, &tm); err != nil {
		return err
	}
	d.Time = tm.UTC()
	return nil
}
