// Package metrics calculates headcount, turnover, retention and tenure. Every
// figure is derived from employment dates and completed separations, so it can
// be recalculated and explained. The definitions here are the single source
// used by the dashboard, the analytics endpoints and reports.
package metrics

import (
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
)

// Person is one employment period, reduced to what the calculations need.
type Person struct {
	ID          bson.ObjectID
	Join        domain.Date
	End         *domain.Date // last day employed; nil while employed
	Assignments []domain.DepartmentAssignment
}

// Exit is one completed separation. Date is the last working day.
type Exit struct {
	EmployeeID bson.ObjectID
	Date       domain.Date
	Category   string
	ReasonCode string
	Department bson.ObjectID
	TenureDays int
}

// Dataset holds the people and exits that count toward metrics. Employment
// types the organization excludes from headcount are filtered out before this
// is built, so numerators and denominators always cover the same population.
type Dataset struct {
	People []Person
	Exits  []Exit
}

// Scope limits a calculation to some departments. A nil scope is the whole organization.
type Scope map[bson.ObjectID]bool

func ScopeOf(ids ...bson.ObjectID) Scope {
	s := Scope{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

const (
	FlagNoHeadcount          = "no_headcount"           // denominator is zero; the rate is not defined
	FlagSmallGroup           = "small_group"            // denominator below the organization's threshold
	FlagBeforeAnalyticsStart = "before_analytics_start" // period starts before the data is complete
	FlagPartialPeriod        = "partial_period"         // period has not finished yet
)

// Rate is a percentage shown together with the counts behind it. Value is nil
// when the rate cannot or should not be stated; Flags says why.
type Rate struct {
	Value       *float64 `json:"value"`
	Numerator   int      `json:"numerator"`
	Denominator float64  `json:"denominator"`
	Flags       []string `json:"flags"`
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// NewRate builds numerator / denominator × 100. A zero denominator gives no
// value rather than 0%, and a small denominator is flagged.
func NewRate(numerator int, denominator float64, smallGroupThreshold int) Rate {
	r := Rate{Numerator: numerator, Denominator: round(denominator, 2), Flags: []string{}}
	if denominator <= 0 {
		r.Flags = append(r.Flags, FlagNoHeadcount)
		return r
	}
	v := round(float64(numerator)/denominator*100, 2)
	r.Value = &v
	if denominator < float64(smallGroupThreshold) {
		r.Flags = append(r.Flags, FlagSmallGroup)
	}
	return r
}

// Withhold removes the value and records the reason, keeping the counts visible.
func (r Rate) Withhold(flag string) Rate {
	r.Value = nil
	r.Flags = append(slices.Clone(r.Flags), flag)
	return r
}

func (r Rate) With(flag string) Rate {
	r.Flags = append(slices.Clone(r.Flags), flag)
	return r
}

// Days is the number of calendar days from a to b, counting both.
func Days(a, b domain.Date) int {
	if b.Before(a) {
		return 0
	}
	return int(b.Sub(a.Time).Hours()/24) + 1
}

func maxDate(a, b domain.Date) domain.Date {
	if a.After(b) {
		return a
	}
	return b
}

func minDate(a, b domain.Date) domain.Date {
	if a.Before(b) {
		return a
	}
	return b
}

// employedOn: a person counts as employed on their last working day.
func (p Person) employedOn(d domain.Date) bool {
	return !p.Join.After(d) && (p.End == nil || !p.End.Before(d))
}

// departmentOn returns the department the person belonged to on a date.
func (p Person) departmentOn(d domain.Date) (bson.ObjectID, bool) {
	for _, a := range p.Assignments {
		if !a.From.After(d) && (a.To == nil || !a.To.Before(d)) {
			return a.DepartmentID, true
		}
	}
	return bson.ObjectID{}, false
}

// daysIn counts the days of [from, to] on which the person was employed and,
// when a scope is given, assigned to a department in that scope.
func (p Person) daysIn(from, to domain.Date, scope Scope) int {
	start, end := maxDate(from, p.Join), to
	if p.End != nil {
		end = minDate(end, *p.End)
	}
	if scope == nil {
		return Days(start, end)
	}
	total := 0
	for _, a := range p.Assignments {
		if !scope[a.DepartmentID] {
			continue
		}
		aEnd := end
		if a.To != nil {
			aEnd = minDate(aEnd, *a.To)
		}
		total += Days(maxDate(start, a.From), aEnd)
	}
	return total
}

func (p Person) inScopeOn(d domain.Date, scope Scope) bool {
	if !p.employedOn(d) {
		return false
	}
	if scope == nil {
		return true
	}
	dept, ok := p.departmentOn(d)
	return ok && scope[dept]
}

// Headcount is the number of people employed on a date.
func (ds Dataset) Headcount(on domain.Date, scope Scope) int {
	n := 0
	for _, p := range ds.People {
		if p.inScopeOn(on, scope) {
			n++
		}
	}
	return n
}

// AverageHeadcount is the mean of the daily headcount over every calendar day
// of the period. It is computed as total person-days divided by days, which is
// the same number without looping over each day.
func (ds Dataset) AverageHeadcount(from, to domain.Date, scope Scope) float64 {
	days := Days(from, to)
	if days == 0 {
		return 0
	}
	personDays := 0
	for _, p := range ds.People {
		personDays += p.daysIn(from, to, scope)
	}
	return float64(personDays) / float64(days)
}

// ExitsIn returns exits whose last working day is in the period. With a scope,
// an exit belongs to the department the employee left from.
func (ds Dataset) ExitsIn(from, to domain.Date, scope Scope) []Exit {
	out := []Exit{}
	for _, e := range ds.Exits {
		if e.Date.Before(from) || e.Date.After(to) || (scope != nil && !scope[e.Department]) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Turnover holds the total rate and its split by category for one period.
// Voluntary + involuntary + other separations always equal the total.
type Turnover struct {
	AverageHeadcount float64 `json:"averageHeadcount"`
	Total            Rate    `json:"total"`
	Voluntary        Rate    `json:"voluntary"`
	Involuntary      Rate    `json:"involuntary"`
	Other            Rate    `json:"other"`
}

// Turnover = separations in the period ÷ average headcount in the period × 100.
func (ds Dataset) Turnover(from, to domain.Date, scope Scope, smallGroup int) Turnover {
	avg := ds.AverageHeadcount(from, to, scope)
	counts := map[string]int{}
	exits := ds.ExitsIn(from, to, scope)
	for _, e := range exits {
		counts[e.Category]++
	}
	other := len(exits) - counts[domain.CategoryVoluntary] - counts[domain.CategoryInvoluntary]
	return Turnover{
		AverageHeadcount: round(avg, 2),
		Total:            NewRate(len(exits), avg, smallGroup),
		Voluntary:        NewRate(counts[domain.CategoryVoluntary], avg, smallGroup),
		Involuntary:      NewRate(counts[domain.CategoryInvoluntary], avg, smallGroup),
		Other:            NewRate(other, avg, smallGroup),
	}
}

func (t Turnover) apply(f func(Rate) Rate) Turnover {
	t.Total, t.Voluntary, t.Involuntary, t.Other = f(t.Total), f(t.Voluntary), f(t.Involuntary), f(t.Other)
	return t
}

// Withhold removes every value in the split, keeping counts.
func (t Turnover) Withhold(flag string) Turnover {
	return t.apply(func(r Rate) Rate { return r.Withhold(flag) })
}

func (t Turnover) With(flag string) Turnover {
	return t.apply(func(r Rate) Rate { return r.With(flag) })
}

// Retention = people employed on the first day who are still employed on the
// last day ÷ people employed on the first day × 100. People hired during the
// period are in neither figure. With a scope, the starting group is those in
// the scoped departments on the first day; moving to another department is
// not leaving.
func (ds Dataset) Retention(from, to domain.Date, scope Scope, smallGroup int) Rate {
	start, stayed := 0, 0
	for _, p := range ds.People {
		if !p.inScopeOn(from, scope) {
			continue
		}
		start++
		if p.employedOn(to) {
			stayed++
		}
	}
	return NewRate(stayed, float64(start), smallGroup)
}

// Month is one calendar month of the headcount and turnover series.
type Month struct {
	Period   string      `json:"period"` // YYYY-MM
	From     domain.Date `json:"from"`
	To       domain.Date `json:"to"`
	Opening  int         `json:"openingHeadcount"` // employed on the day before the month starts
	Joiners  int         `json:"joiners"`
	Leavers  int         `json:"leavers"`
	Closing  int         `json:"closingHeadcount"`
	Net      int         `json:"netChange"`
	Turnover Turnover    `json:"turnover"`
	Partial  bool        `json:"partial"`
}

// firstOfMonth and lastOfMonth bound the calendar month containing d.
func firstOfMonth(d domain.Date) domain.Date { return domain.NewDate(d.Year(), d.Month(), 1) }
func lastOfMonth(d domain.Date) domain.Date {
	return domain.Date{Time: firstOfMonth(d).AddDate(0, 1, -1)}
}

// Monthly returns one entry per calendar month touched by [from, to]. A month
// that extends past `to` (normally today) is cut there and marked partial.
// For whole-organization figures, closing = opening + joiners − leavers.
func (ds Dataset) Monthly(from, to domain.Date, scope Scope, smallGroup int) []Month {
	out := []Month{}
	for start := firstOfMonth(from); !start.After(to); start = (domain.Date{Time: start.AddDate(0, 1, 0)}) {
		end, partial := lastOfMonth(start), false
		if end.After(to) {
			end, partial = to, true
		}
		m := Month{Period: start.Format("2006-01"), From: start, To: end, Partial: partial,
			Opening: ds.Headcount(start.AddDays(-1), scope), Closing: ds.Headcount(end, scope)}
		for _, p := range ds.People {
			// A joiner or leaver is attributed to the department they were in on that day.
			if !p.Join.Before(start) && !p.Join.After(end) && p.inScopeOn(p.Join, scope) {
				m.Joiners++
			}
			if p.End != nil && !p.End.Before(start) && !p.End.After(end) && p.inScopeOn(*p.End, scope) {
				m.Leavers++
			}
		}
		m.Net = m.Closing - m.Opening
		m.Turnover = ds.Turnover(start, end, scope, smallGroup)
		if partial {
			m.Turnover = m.Turnover.With(FlagPartialPeriod)
		}
		out = append(out, m)
	}
	return out
}

// Transfers counts moves into and out of the scoped departments during the
// period. They are reported beside turnover and are never part of it.
func (ds Dataset) Transfers(from, to domain.Date, scope Scope) (in, out int) {
	for _, p := range ds.People {
		for i := 1; i < len(p.Assignments); i++ {
			prev, cur := p.Assignments[i-1], p.Assignments[i]
			if cur.From.Before(from) || cur.From.After(to) {
				continue
			}
			if scope[cur.DepartmentID] && !scope[prev.DepartmentID] {
				in++
			}
			if scope[prev.DepartmentID] && !scope[cur.DepartmentID] {
				out++
			}
		}
	}
	return in, out
}

// TenureStats summarizes lengths of service. The median is given beside the
// mean because a few long-serving people can pull the mean up.
type TenureStats struct {
	Count       int            `json:"count"`
	MeanYears   *float64       `json:"meanYears"`
	MedianYears *float64       `json:"medianYears"`
	Bands       map[string]int `json:"bands"`
}

// TenureBands are the groupings used for tenure distributions, in order.
var TenureBands = []string{"under_6_months", "6_to_12_months", "1_to_2_years", "2_to_5_years", "over_5_years"}

func BandOf(days int) string {
	switch {
	case days < 183:
		return TenureBands[0]
	case days < 365:
		return TenureBands[1]
	case days < 730:
		return TenureBands[2]
	case days < 1826:
		return TenureBands[3]
	}
	return TenureBands[4]
}

func summarize(days []int) TenureStats {
	s := TenureStats{Count: len(days), Bands: map[string]int{}}
	for _, b := range TenureBands {
		s.Bands[b] = 0
	}
	if len(days) == 0 {
		return s
	}
	slices.Sort(days)
	sum := 0
	for _, d := range days {
		sum += d
		s.Bands[BandOf(d)]++
	}
	median := float64(days[len(days)/2])
	if len(days)%2 == 0 {
		median = float64(days[len(days)/2-1]+days[len(days)/2]) / 2
	}
	mean, med := round(float64(sum)/float64(len(days))/365.25, 1), round(median/365.25, 1)
	s.MeanYears, s.MedianYears = &mean, &med
	return s
}

// CurrentTenure is the length of service, as of a date, of everyone employed on it.
func (ds Dataset) CurrentTenure(asOf domain.Date, scope Scope) TenureStats {
	days := []int{}
	for _, p := range ds.People {
		if p.inScopeOn(asOf, scope) {
			days = append(days, Days(p.Join, asOf)-1)
		}
	}
	return summarize(days)
}

// LeaverTenure is the length of service of people whose last working day is in the period.
func (ds Dataset) LeaverTenure(from, to domain.Date, scope Scope) TenureStats {
	days := []int{}
	for _, e := range ds.ExitsIn(from, to, scope) {
		days = append(days, e.TenureDays)
	}
	return summarize(days)
}

// ClipToToday shortens a period that runs into the future and reports whether it did.
func ClipToToday(to, today domain.Date) (domain.Date, bool) {
	if to.After(today) {
		return today, true
	}
	return to, false
}

// MonthsBack returns the first day of the month n months before the month of d.
func MonthsBack(d domain.Date, n int) domain.Date {
	return domain.Date{Time: time.Date(d.Year(), d.Month()-time.Month(n), 1, 0, 0, 0, 0, time.UTC)}
}
