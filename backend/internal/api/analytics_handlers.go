package api

import (
	"context"
	"net/http"
	"slices"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/metrics"
)

const (
	warnNoAnalyticsStart = "analytics_start_date_not_set"
	warnPeriodClipped    = "period_ends_in_future_clipped_to_today"
	warnBeforeStart      = "period_starts_before_analytics_start_date"
	maxAnalyticsYears    = 5
)

// analysis is everything an analytics endpoint needs for one request: the
// data, the period, what the caller is allowed to see and the caveats.
type analysis struct {
	org      *domain.Organization
	today    domain.Date
	from, to domain.Date
	ds       metrics.Dataset
	scope    metrics.Scope // nil means the whole organization
	depts    []domain.Department
	small    int
	warnings []string
}

func (a *analysis) envelope(extra map[string]any) map[string]any {
	out := map[string]any{"from": a.from, "to": a.to, "analyticsStartDate": a.org.Settings.AnalyticsStartDate,
		"smallGroupThreshold": a.small, "scoped": a.scope != nil, "warnings": a.warnings}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// beforeStart reports whether a period begins before the data is known to be complete.
func (a *analysis) beforeStart(from domain.Date) bool {
	start := a.org.Settings.AnalyticsStartDate
	return start != nil && from.Before(*start)
}

// guardRate withholds a rate for a period that starts before the analytics start date.
func (a *analysis) guardRate(r metrics.Rate, from domain.Date) metrics.Rate {
	if a.beforeStart(from) {
		return r.Withhold(metrics.FlagBeforeAnalyticsStart)
	}
	return r
}

func (a *analysis) guardTurnover(t metrics.Turnover, from domain.Date) metrics.Turnover {
	if a.beforeStart(from) {
		return t.Withhold(metrics.FlagBeforeAnalyticsStart)
	}
	return t
}

// newAnalysis reads the period and department filter, applies the caller's
// department scope and loads the dataset. defaultMonths sets the period used
// when none is given: that many calendar months ending today.
func (s *Server) newAnalysis(r *http.Request, defaultMonths int) (*analysis, error) {
	orgID := principal(r).User.OrganizationID
	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		return nil, err
	}
	a := &analysis{org: org, today: today, from: metrics.MonthsBack(today, defaultMonths-1), to: today,
		small: org.SmallGroupThreshold(), warnings: []string{}}
	q := r.URL.Query()
	if raw := q.Get("from"); raw != "" {
		if a.from, err = domain.ParseDate(raw); err != nil {
			return nil, httpx.BadRequest("from " + err.Error() + ".")
		}
	}
	if raw := q.Get("to"); raw != "" {
		if a.to, err = domain.ParseDate(raw); err != nil {
			return nil, httpx.BadRequest("to " + err.Error() + ".")
		}
	}
	if a.from.After(a.to) || a.to.After(a.from.AddYears(maxAnalyticsYears)) {
		return nil, httpx.BadRequest("from must not be after to, and the range can be at most five years.")
	}
	if a.from.After(today) {
		return nil, httpx.BadRequest("from cannot be in the future.")
	}
	var clipped bool
	if a.to, clipped = metrics.ClipToToday(a.to, today); clipped {
		a.warnings = append(a.warnings, warnPeriodClipped)
	}
	if org.Settings.AnalyticsStartDate == nil {
		a.warnings = append(a.warnings, warnNoAnalyticsStart)
	} else if a.beforeStart(a.from) {
		a.warnings = append(a.warnings, warnBeforeStart)
	}

	// Department scope: department heads see only their departments.
	all, allowed, err := s.employeeScope(r)
	if err != nil {
		return nil, err
	}
	if !all {
		a.scope = metrics.ScopeOf(allowed...)
	}
	if raw := q.Get("departmentId"); raw != "" {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			return nil, httpx.BadRequest("departmentId is not valid.")
		}
		if all || slices.Contains(allowed, id) {
			a.scope = metrics.ScopeOf(id)
		} else {
			a.scope = metrics.Scope{}
		}
	}

	if a.depts, err = s.store.ListDepartments(r.Context(), orgID, ""); err != nil {
		return nil, err
	}
	if a.ds, err = s.loadDataset(r.Context(), org); err != nil {
		return nil, err
	}
	return a, nil
}

// loadDataset reads the people and exits that count toward an organization's metrics.
func (s *Server) loadDataset(ctx context.Context, org *domain.Organization) (metrics.Dataset, error) {
	var ds metrics.Dataset
	counted := []string{}
	for _, t := range org.EmploymentTypes() {
		if t.CountsInHeadcount {
			counted = append(counted, t.Code)
		}
	}
	var err error
	if ds.People, err = s.store.MetricPeople(ctx, org.ID, counted); err != nil {
		return ds, err
	}
	exits, err := s.store.CompletedExits(ctx, org.ID)
	if err != nil {
		return ds, err
	}
	// Keep only exits of counted people, so numerator and denominator cover the same population.
	ids := make(map[bson.ObjectID]bool, len(ds.People))
	for _, p := range ds.People {
		ids[p.ID] = true
	}
	for _, e := range exits {
		if ids[e.EmployeeID] {
			ds.Exits = append(ds.Exits, e)
		}
	}
	return ds, nil
}

// visibleDepartments lists the departments the analysis covers.
func (a *analysis) visibleDepartments() []domain.Department {
	if a.scope == nil {
		return a.depts
	}
	return slices.DeleteFunc(slices.Clone(a.depts), func(d domain.Department) bool { return !a.scope[d.ID] })
}

func (a *analysis) guardMonths(months []metrics.Month) []metrics.Month {
	for i := range months {
		months[i].Turnover = a.guardTurnover(months[i].Turnover, months[i].From)
	}
	return months
}

type departmentTurnover struct {
	DepartmentID bson.ObjectID    `json:"departmentId"`
	Name         string           `json:"name"`
	Code         string           `json:"code"`
	Archived     bool             `json:"archived"`
	Turnover     metrics.Turnover `json:"turnover"`
	TransfersIn  int              `json:"transfersIn"`
	TransfersOut int              `json:"transfersOut"`
}

type reasonCount struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// handleTurnover returns turnover for the period: overall, the rolling 12
// months, each calendar month, each department, and counts by reason and tenure.
func (s *Server) handleTurnover(w http.ResponseWriter, r *http.Request) {
	a, err := s.newAnalysis(r, 12)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rollingFrom := a.today.AddYears(-1).AddDays(1)
	byDept := []departmentTurnover{}
	for _, d := range a.visibleDepartments() {
		t := a.ds.Turnover(a.from, a.to, metrics.ScopeOf(d.ID), a.small)
		if d.Status == domain.DepartmentArchived && t.AverageHeadcount == 0 {
			continue // an archived department with nobody in the period is noise
		}
		in, out := a.ds.Transfers(a.from, a.to, metrics.ScopeOf(d.ID))
		byDept = append(byDept, departmentTurnover{DepartmentID: d.ID, Name: d.Name, Code: d.Code,
			Archived: d.Status == domain.DepartmentArchived, Turnover: a.guardTurnover(t, a.from), TransfersIn: in, TransfersOut: out})
	}

	exits := a.ds.ExitsIn(a.from, a.to, a.scope)
	names := map[string]domain.SeparationReason{}
	for _, reason := range a.org.SeparationReasons() {
		names[reason.Code] = reason
	}
	counts, bands := map[string]int{}, map[string]int{}
	for _, b := range metrics.TenureBands {
		bands[b] = 0
	}
	for _, e := range exits {
		counts[e.ReasonCode]++
		bands[metrics.BandOf(e.TenureDays)]++
	}
	byReason := []reasonCount{}
	for code, n := range counts {
		rc := reasonCount{Code: code, Name: code, Count: n}
		if known, ok := names[code]; ok {
			rc.Name, rc.Category = known.Name, known.Category
		}
		byReason = append(byReason, rc)
	}
	sort.Slice(byReason, func(i, j int) bool {
		if byReason[i].Count != byReason[j].Count {
			return byReason[i].Count > byReason[j].Count
		}
		return byReason[i].Code < byReason[j].Code
	})

	httpx.JSON(w, 200, a.envelope(map[string]any{
		"overall":          a.guardTurnover(a.ds.Turnover(a.from, a.to, a.scope, a.small), a.from),
		"rolling12Months":  map[string]any{"from": rollingFrom, "to": a.today, "turnover": a.guardTurnover(a.ds.Turnover(rollingFrom, a.today, a.scope, a.small), rollingFrom)},
		"monthly":          a.guardMonths(a.ds.Monthly(a.from, a.to, a.scope, a.small)),
		"byDepartment":     byDept,
		"byReason":         byReason,
		"byTenureBand":     bands,
		"separationsCount": len(exits),
	}))
}

// handleHeadcount returns the current headcount and the monthly movement.
func (s *Server) handleHeadcount(w http.ResponseWriter, r *http.Request) {
	a, err := s.newAnalysis(r, 12)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type deptCount struct {
		DepartmentID bson.ObjectID `json:"departmentId"`
		Name         string        `json:"name"`
		Code         string        `json:"code"`
		Headcount    int           `json:"headcount"`
	}
	byDept := []deptCount{}
	for _, d := range a.visibleDepartments() {
		if n := a.ds.Headcount(a.today, metrics.ScopeOf(d.ID)); n > 0 || d.Status == domain.DepartmentActive {
			byDept = append(byDept, deptCount{d.ID, d.Name, d.Code, n})
		}
	}
	httpx.JSON(w, 200, a.envelope(map[string]any{
		"currentHeadcount": a.ds.Headcount(a.today, a.scope),
		"asOf":             a.today,
		"monthly":          a.guardMonths(a.ds.Monthly(a.from, a.to, a.scope, a.small)),
		"byDepartment":     byDept,
	}))
}

// handleRetention returns the retention rate for the period, overall and by department.
func (s *Server) handleRetention(w http.ResponseWriter, r *http.Request) {
	a, err := s.newAnalysis(r, 12)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type deptRetention struct {
		DepartmentID bson.ObjectID `json:"departmentId"`
		Name         string        `json:"name"`
		Code         string        `json:"code"`
		Retention    metrics.Rate  `json:"retention"`
	}
	byDept := []deptRetention{}
	for _, d := range a.visibleDepartments() {
		rate := a.ds.Retention(a.from, a.to, metrics.ScopeOf(d.ID), a.small)
		if rate.Denominator == 0 {
			continue // nobody was in the department on the first day
		}
		byDept = append(byDept, deptRetention{d.ID, d.Name, d.Code, a.guardRate(rate, a.from)})
	}
	httpx.JSON(w, 200, a.envelope(map[string]any{
		"retention":    a.guardRate(a.ds.Retention(a.from, a.to, a.scope, a.small), a.from),
		"byDepartment": byDept,
	}))
}

// handleTenure returns length of service for current staff and for leavers in the period.
func (s *Server) handleTenure(w http.ResponseWriter, r *http.Request) {
	a, err := s.newAnalysis(r, 12)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, a.envelope(map[string]any{
		"asOf":         a.to,
		"currentStaff": a.ds.CurrentTenure(a.to, a.scope),
		"leavers":      a.ds.LeaverTenure(a.from, a.to, a.scope),
		"bands":        metrics.TenureBands,
	}))
}

type qualityIssue struct {
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Count       int      `json:"count"`
	Examples    []string `json:"examples"` // up to ten employee numbers
}

// handleDataQuality lists problems in the records that would distort metrics.
func (s *Server) handleDataQuality(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := s.store.EmployeeQualityRows(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	issues := map[string]*qualityIssue{}
	order := []string{}
	add := func(code, description, employeeNumber string) {
		issue, ok := issues[code]
		if !ok {
			issue = &qualityIssue{Code: code, Description: description, Examples: []string{}}
			issues[code] = issue
			order = append(order, code)
		}
		issue.Count++
		if len(issue.Examples) < 10 {
			issue.Examples = append(issue.Examples, employeeNumber)
		}
	}
	for _, e := range rows {
		switch {
		case e.End != nil && e.End.Before(e.Join):
			add("end_before_join", "End date is before the join date.", e.EmployeeNumber)
		case e.Status == domain.EmployeeSeparated && e.End == nil:
			add("separated_without_end_date", "Status is separated but there is no end date, so the person still counts in headcount.", e.EmployeeNumber)
		case e.Status != domain.EmployeeSeparated && e.End != nil:
			add("end_date_without_separation", "There is an end date but the status is not separated.", e.EmployeeNumber)
		}
		if len(e.History) == 0 {
			add("no_department_history", "No department history, so the person is missing from department figures.", e.EmployeeNumber)
		} else if e.History[0].From.After(e.Join) {
			add("department_history_gap", "Department history starts after the join date, leaving days with no department.", e.EmployeeNumber)
		}
		if e.Join.After(today) {
			add("future_join_date", "Join date is in the future; the person is not counted until then.", e.EmployeeNumber)
		}
	}
	out := []qualityIssue{}
	for _, code := range order {
		out = append(out, *issues[code])
	}
	httpx.JSON(w, 200, map[string]any{
		"analyticsStartDate":    org.Settings.AnalyticsStartDate,
		"analyticsStartDateSet": org.Settings.AnalyticsStartDate != nil,
		"employeeRecords":       len(rows),
		"issues":                out,
	})
}
