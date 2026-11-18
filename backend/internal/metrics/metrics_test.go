package metrics

import (
	"math"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
)

func d(y int, m time.Month, day int) domain.Date { return domain.NewDate(y, m, day) }

func near(got, want float64) bool { return math.Abs(got-want) < 0.005 }

var deptX, deptY = bson.NewObjectID(), bson.NewObjectID()

// sample is a four-person organization, small enough to check by hand.
//
//	A: joined 2025-01-01, still employed, department X throughout
//	B: joined 2025-06-01, last working day 2026-03-10 (voluntary), department X
//	C: joined 2026-03-16, still employed, department Y
//	D: joined 2024-01-01, still employed, X until 2026-03-20, Y from 2026-03-21
//
// Person-days in March 2026 (31 days): A 31, B 10, C 16, D 31 = 88.
// By department: X = 31 + 10 + 20 = 61, Y = 16 + 11 = 27.
func sample() Dataset {
	bEnd, dMove := d(2026, 3, 10), d(2026, 3, 20)
	b := bson.NewObjectID()
	return Dataset{
		People: []Person{
			{ID: bson.NewObjectID(), Join: d(2025, 1, 1), Assignments: []domain.DepartmentAssignment{{DepartmentID: deptX, From: d(2025, 1, 1)}}},
			{ID: b, Join: d(2025, 6, 1), End: &bEnd, Assignments: []domain.DepartmentAssignment{{DepartmentID: deptX, From: d(2025, 6, 1), To: &bEnd}}},
			{ID: bson.NewObjectID(), Join: d(2026, 3, 16), Assignments: []domain.DepartmentAssignment{{DepartmentID: deptY, From: d(2026, 3, 16)}}},
			{ID: bson.NewObjectID(), Join: d(2024, 1, 1), Assignments: []domain.DepartmentAssignment{
				{DepartmentID: deptX, From: d(2024, 1, 1), To: &dMove}, {DepartmentID: deptY, From: d(2026, 3, 21)}}},
		},
		// 2025-06-01 to 2026-03-10 is 282 days.
		Exits: []Exit{{EmployeeID: b, Date: bEnd, Category: domain.CategoryVoluntary, ReasonCode: "personal", Department: deptX, TenureDays: 282}},
	}
}

var march1, march31 = d(2026, 3, 1), d(2026, 3, 31)

func TestHeadcount(t *testing.T) {
	ds := sample()
	for date, want := range map[domain.Date]int{
		d(2023, 12, 31): 0, // before anyone joined
		march1:          3, // A, B, D
		d(2026, 3, 10):  3, // B still counts on the last working day
		d(2026, 3, 11):  2,
		d(2026, 3, 16):  3, // C joins
		march31:         3,
	} {
		if got := ds.Headcount(date, nil); got != want {
			t.Errorf("headcount on %s = %d, want %d", date, got, want)
		}
	}
	// D is in X on the 20th and in Y from the 21st.
	if x, y := ds.Headcount(d(2026, 3, 20), ScopeOf(deptX)), ds.Headcount(d(2026, 3, 20), ScopeOf(deptY)); x != 2 || y != 1 {
		t.Errorf("20 March by department: X=%d Y=%d, want 2 and 1", x, y)
	}
	if x, y := ds.Headcount(d(2026, 3, 21), ScopeOf(deptX)), ds.Headcount(d(2026, 3, 21), ScopeOf(deptY)); x != 1 || y != 2 {
		t.Errorf("21 March by department: X=%d Y=%d, want 1 and 2", x, y)
	}
}

func TestAverageHeadcountIsMeanOfDailyHeadcount(t *testing.T) {
	ds := sample()
	if got := ds.AverageHeadcount(march1, march31, nil); !near(got, 88.0/31) {
		t.Errorf("organization average = %v, want %v", got, 88.0/31)
	}
	x, y := ds.AverageHeadcount(march1, march31, ScopeOf(deptX)), ds.AverageHeadcount(march1, march31, ScopeOf(deptY))
	if !near(x, 61.0/31) || !near(y, 27.0/31) {
		t.Errorf("department averages X=%v Y=%v, want %v and %v", x, y, 61.0/31, 27.0/31)
	}
	// Departments partition the organization, so their averages add up to the total.
	if !near(x+y, ds.AverageHeadcount(march1, march31, nil)) {
		t.Errorf("department averages %v do not add up to the organization average", x+y)
	}
	// Cross-check the shortcut against a literal day-by-day mean.
	sum := 0
	for day := march1; !day.After(march31); day = day.AddDays(1) {
		sum += ds.Headcount(day, ScopeOf(deptX))
	}
	if !near(x, float64(sum)/31) {
		t.Errorf("person-days method %v differs from the daily mean %v", x, float64(sum)/31)
	}
}

func TestTurnover(t *testing.T) {
	ds := sample()
	org := ds.Turnover(march1, march31, nil, 5)
	// 1 separation ÷ 2.8387 average headcount × 100 = 35.23%.
	if org.Total.Value == nil || !near(*org.Total.Value, 35.23) || org.Total.Numerator != 1 || !near(org.Total.Denominator, 2.84) {
		t.Errorf("organization turnover = %+v", org.Total)
	}
	if !near(*org.Voluntary.Value, 35.23) || *org.Involuntary.Value != 0 || *org.Other.Value != 0 {
		t.Errorf("split: voluntary=%v involuntary=%v other=%v", *org.Voluntary.Value, *org.Involuntary.Value, *org.Other.Value)
	}
	if got := org.Voluntary.Numerator + org.Involuntary.Numerator + org.Other.Numerator; got != org.Total.Numerator {
		t.Errorf("categories add up to %d, total is %d", got, org.Total.Numerator)
	}
	if !slices.Contains(org.Total.Flags, FlagSmallGroup) {
		t.Error("an average headcount of 2.8 with threshold 5 should be flagged as a small group")
	}

	// 1 ÷ 1.9677 × 100 = 50.82% for X; nobody left from Y.
	if x := ds.Turnover(march1, march31, ScopeOf(deptX), 1); !near(*x.Total.Value, 50.82) || len(x.Total.Flags) != 0 {
		t.Errorf("department X turnover = %+v", x.Total)
	}
	if y := ds.Turnover(march1, march31, ScopeOf(deptY), 1); *y.Total.Value != 0 || y.Total.Numerator != 0 {
		t.Errorf("department Y turnover = %+v", y.Total)
	}

	// The last working day is the boundary: inside when the period ends on it, outside the day after.
	if got := ds.Turnover(march1, d(2026, 3, 10), nil, 1).Total.Numerator; got != 1 {
		t.Errorf("period ending on the last working day counted %d separations", got)
	}
	if got := ds.Turnover(d(2026, 3, 11), march31, nil, 1).Total.Numerator; got != 0 {
		t.Errorf("period starting the day after counted %d separations", got)
	}
}

func TestZeroHeadcountGivesNoRateNotZero(t *testing.T) {
	r := sample().Turnover(d(2023, 1, 1), d(2023, 1, 31), nil, 5).Total
	if r.Value != nil || !slices.Contains(r.Flags, FlagNoHeadcount) {
		t.Errorf("turnover with nobody employed = %+v, want no value", r)
	}
	withheld := NewRate(3, 40, 5).Withhold(FlagBeforeAnalyticsStart)
	if withheld.Value != nil || withheld.Numerator != 3 || withheld.Denominator != 40 {
		t.Errorf("a withheld rate must keep its counts: %+v", withheld)
	}
}

func TestRetention(t *testing.T) {
	ds := sample()
	// Employed on 1 March: A, B, D. Still employed on 31 March: A, D. C joined mid-month and is excluded.
	r := ds.Retention(march1, march31, nil, 1)
	if !near(*r.Value, 66.67) || r.Numerator != 2 || r.Denominator != 3 {
		t.Errorf("retention = %+v", r)
	}
	// Retention is not 100 minus turnover: 100 − 35.23 = 64.77, not 66.67.
	if near(*r.Value, 100-*ds.Turnover(march1, march31, nil, 1).Total.Value) {
		t.Error("retention should differ from 100 minus turnover in this data")
	}
	// D moved from X to Y during the month. That is not leaving the organization.
	if x := ds.Retention(march1, march31, ScopeOf(deptX), 1); x.Numerator != 2 || x.Denominator != 3 {
		t.Errorf("department X retention = %+v", x)
	}
	if y := ds.Retention(march1, march31, ScopeOf(deptY), 1); y.Value != nil {
		t.Errorf("department Y had nobody on 1 March, so retention has no value: %+v", y)
	}
}

func TestMonthlySeries(t *testing.T) {
	ds := sample()
	months := ds.Monthly(d(2026, 2, 1), march31, nil, 1)
	if len(months) != 2 || months[0].Period != "2026-02" || months[1].Period != "2026-03" {
		t.Fatalf("months = %+v", months)
	}
	feb, mar := months[0], months[1]
	if feb.Opening != 3 || feb.Joiners != 0 || feb.Leavers != 0 || feb.Closing != 3 {
		t.Errorf("February = %+v", feb)
	}
	if mar.Opening != 3 || mar.Joiners != 1 || mar.Leavers != 1 || mar.Closing != 3 || mar.Partial {
		t.Errorf("March = %+v", mar)
	}
	for _, m := range months {
		if m.Opening+m.Joiners-m.Leavers != m.Closing {
			t.Errorf("%s: opening %d + joiners %d − leavers %d ≠ closing %d", m.Period, m.Opening, m.Joiners, m.Leavers, m.Closing)
		}
	}
	if !near(*mar.Turnover.Total.Value, 35.23) {
		t.Errorf("March turnover in the series = %v", *mar.Turnover.Total.Value)
	}

	// A month that has not finished is cut at "today" and marked partial.
	partial := ds.Monthly(march1, d(2026, 3, 20), nil, 1)
	if len(partial) != 1 || !partial[0].Partial || partial[0].To != d(2026, 3, 20) || !slices.Contains(partial[0].Turnover.Total.Flags, FlagPartialPeriod) {
		t.Errorf("partial month = %+v", partial)
	}
}

func TestTransfersAreNotTurnover(t *testing.T) {
	ds := sample()
	if in, out := ds.Transfers(march1, march31, ScopeOf(deptX)); in != 0 || out != 1 {
		t.Errorf("X transfers in=%d out=%d, want 0 and 1", in, out)
	}
	if in, out := ds.Transfers(march1, march31, ScopeOf(deptY)); in != 1 || out != 0 {
		t.Errorf("Y transfers in=%d out=%d, want 1 and 0", in, out)
	}
	// D's move out of X must not appear as a separation from X.
	if n := len(ds.ExitsIn(march1, march31, ScopeOf(deptX))); n != 1 {
		t.Errorf("X separations = %d, want only B", n)
	}
}

func TestTenure(t *testing.T) {
	ds := sample()
	// As of 31 March 2026: C 15 days, A 454 days, D 820 days.
	cur := ds.CurrentTenure(march31, nil)
	if cur.Count != 3 || *cur.MedianYears != 1.2 || *cur.MeanYears != 1.2 { // 454/365.25 = 1.24; (1289/3)/365.25 = 1.18
		t.Errorf("current tenure = count %d mean %v median %v", cur.Count, *cur.MeanYears, *cur.MedianYears)
	}
	if cur.Bands["under_6_months"] != 1 || cur.Bands["1_to_2_years"] != 1 || cur.Bands["2_to_5_years"] != 1 {
		t.Errorf("current tenure bands = %v", cur.Bands)
	}
	left := ds.LeaverTenure(march1, march31, nil)
	if left.Count != 1 || *left.MeanYears != 0.8 || left.Bands["6_to_12_months"] != 1 { // 282/365.25 = 0.77
		t.Errorf("leaver tenure = %+v", left)
	}
	if empty := ds.LeaverTenure(d(2023, 1, 1), d(2023, 12, 31), nil); empty.Count != 0 || empty.MeanYears != nil {
		t.Errorf("no leavers should give no mean: %+v", empty)
	}
	for days, want := range map[int]string{0: "under_6_months", 182: "under_6_months", 183: "6_to_12_months", 364: "6_to_12_months",
		365: "1_to_2_years", 729: "1_to_2_years", 730: "2_to_5_years", 1825: "2_to_5_years", 1826: "over_5_years"} {
		if got := BandOf(days); got != want {
			t.Errorf("BandOf(%d) = %s, want %s", days, got, want)
		}
	}
}

func TestPeriodHelpers(t *testing.T) {
	if got := Days(march1, march31); got != 31 {
		t.Errorf("Days(March) = %d", got)
	}
	if got := Days(march31, march1); got != 0 {
		t.Errorf("Days of a reversed period = %d, want 0", got)
	}
	if got := MonthsBack(d(2026, 3, 18), 11); got != d(2025, 4, 1) {
		t.Errorf("MonthsBack = %s, want 2025-04-01", got)
	}
	if to, clipped := ClipToToday(march31, d(2026, 3, 18)); to != d(2026, 3, 18) || !clipped {
		t.Errorf("ClipToToday = %s %v", to, clipped)
	}
}
