package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/metrics"
	"peoplepulse/internal/store"
)

// table is a report: a header row and data rows, written as CSV or JSON.
type table struct {
	Title   string     `json:"title"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	// Notes are caveats a reader needs, such as the period or withheld values.
	Notes []string `json:"notes"`
}

var reportTypes = []string{"employees", "separations", "clearance", "turnover-summary"}

// csvSafe stops a spreadsheet from treating a cell as a formula. Values that
// start with =, +, - or @ are prefixed with an apostrophe.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func optDate(d *domain.Date) string {
	if d == nil {
		return ""
	}
	return d.String()
}

func rateCell(r metrics.Rate) string {
	if r.Value == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*r.Value, 'f', 2, 64)
}

// handleReport builds one of the standard reports. ?format=csv downloads it;
// otherwise it is returned as JSON for display. Exports are audited.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	kind := chi.URLParam(r, "type")
	if !slices.Contains(reportTypes, kind) {
		httpx.Fail(w, r, httpx.NotFound("Report"))
		return
	}
	// Period and department scope are read the same way as for analytics.
	a, err := s.newAnalysis(r, 12)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	period := fmt.Sprintf("Period: %s to %s", a.from, a.to)
	deptName := map[bson.ObjectID]string{}
	for _, d := range a.depts {
		deptName[d.ID] = d.Name
	}
	var scopeIDs []bson.ObjectID
	if a.scope != nil {
		scopeIDs = []bson.ObjectID{}
		for id := range a.scope {
			scopeIDs = append(scopeIDs, id)
		}
	}
	inScope := func(dept bson.ObjectID) bool { return a.scope == nil || a.scope[dept] }

	var t table
	switch kind {
	case "employees":
		list, err := s.store.ReportEmployees(r.Context(), a.org.ID, scopeIDs)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		types := map[string]string{}
		for _, et := range a.org.EmploymentTypes() {
			types[et.Code] = et.Name
		}
		t = table{Title: "Employee register", Notes: []string{"All employees, including those who have left."},
			Columns: []string{"Employee number", "Full name", "Department", "Job title", "Employment type", "Join date", "End date", "Status", "Work email"}}
		for _, e := range list {
			t.Rows = append(t.Rows, []string{e.EmployeeNumber, e.FullName, deptName[e.DepartmentID], e.JobTitle, types[e.EmploymentType],
				e.JoinDate.String(), optDate(e.EndDate), e.Status, e.WorkEmail})
		}

	case "separations":
		list, err := s.store.ReportSeparations(r.Context(), a.org.ID, a.from, a.to)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		ids := make([]bson.ObjectID, len(list))
		for i := range list {
			ids[i] = list[i].EmployeeID
		}
		emps, err := s.store.EmployeesByIDs(r.Context(), a.org.ID, ids)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		reasons := map[string]string{}
		for _, reason := range a.org.SeparationReasons() {
			reasons[reason.Code] = reason.Name
		}
		// The free-text reason details are deliberately not part of the export.
		t = table{Title: "Separation register", Notes: []string{period + ", by last working day.", "Free-text reason details are not included."},
			Columns: []string{"Employee number", "Full name", "Department", "Type", "Reason", "Category", "Submitted", "Last working day", "Status", "Tenure (days)"}}
		for _, sep := range list {
			e := emps[sep.EmployeeID]
			dept := e.DepartmentID
			if sep.DepartmentIDAtExit != nil {
				dept = *sep.DepartmentIDAtExit
			}
			if !inScope(dept) {
				continue
			}
			tenure := ""
			if sep.TenureDaysAtExit != nil {
				tenure = strconv.Itoa(*sep.TenureDaysAtExit)
			}
			t.Rows = append(t.Rows, []string{e.EmployeeNumber, e.FullName, deptName[dept], sep.Type, reasons[sep.ReasonCode], sep.Category,
				sep.SubmissionDate.String(), sep.LastWorkingDay.String(), sep.Status, tenure})
		}

	case "clearance":
		if !p.Can(domain.PermClearanceRead) && !p.Can(domain.PermClearanceManage) {
			httpx.Fail(w, r, httpx.Forbidden())
			return
		}
		list, err := s.store.ReportClearances(r.Context(), a.org.ID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		views, err := s.clearanceViews(r.Context(), a.org.ID, list)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		t = table{Title: "Clearance status", Notes: []string{"One row per checklist item, for every checklist."},
			Columns: []string{"Employee number", "Full name", "Last working day", "Checklist status", "Category", "Item", "Item status", "Due date", "Overdue", "Remarks"}}
		for _, v := range views {
			if v.Employee == nil || !inScope(v.Employee.DepartmentID) {
				continue
			}
			for _, item := range v.Items {
				overdue := "no"
				if item.Open() && v.Status == domain.ClearanceInProgress && item.DueDate.Before(a.today) {
					overdue = "yes"
				}
				t.Rows = append(t.Rows, []string{v.Employee.EmployeeNumber, v.Employee.FullName, optDate(v.LastWorkingDay), v.Status,
					item.Category, item.Title, item.Status, item.DueDate.String(), overdue, item.Remarks})
			}
		}

	case "turnover-summary":
		if !p.Can(domain.PermAnalyticsRead) {
			httpx.Fail(w, r, httpx.Forbidden())
			return
		}
		t = table{Title: "Turnover summary", Notes: []string{period + ".", "Rates are percentages: separations ÷ average headcount × 100.",
			"n/a means the rate is not stated: nobody was employed, or the month starts before the analytics start date."},
			Columns: []string{"Month", "Opening headcount", "Joiners", "Leavers", "Closing headcount", "Average headcount", "Separations",
				"Turnover %", "Voluntary %", "Involuntary %", "Other %", "Flags"}}
		for _, m := range a.guardMonths(a.ds.Monthly(a.from, a.to, a.scope, a.small)) {
			t.Rows = append(t.Rows, []string{m.Period, strconv.Itoa(m.Opening), strconv.Itoa(m.Joiners), strconv.Itoa(m.Leavers), strconv.Itoa(m.Closing),
				strconv.FormatFloat(m.Turnover.AverageHeadcount, 'f', 2, 64), strconv.Itoa(m.Turnover.Total.Numerator),
				rateCell(m.Turnover.Total), rateCell(m.Turnover.Voluntary), rateCell(m.Turnover.Involuntary), rateCell(m.Turnover.Other),
				strings.Join(m.Turnover.Total.Flags, " ")})
		}
		if len(a.warnings) > 0 {
			t.Notes = append(t.Notes, "Warnings: "+strings.Join(a.warnings, ", ")+".")
		}
	}
	if t.Rows == nil {
		t.Rows = [][]string{}
	}
	if len(t.Rows) >= store.MaxReportRows {
		t.Notes = append(t.Notes, fmt.Sprintf("The report was cut at %d rows. Narrow the period or filter by department.", store.MaxReportRows))
	}

	if r.URL.Query().Get("format") != "csv" {
		httpx.JSON(w, 200, map[string]any{"type": kind, "from": a.from, "to": a.to, "generatedAt": time.Now().UTC(), "report": t})
		return
	}
	s.audit(r, a.org.ID, &p.User.ID, "report.exported", "report", kind,
		map[string]any{"rows": len(t.Rows), "from": a.from.String(), "to": a.to.String(), "scoped": a.scope != nil})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="peoplepulse-%s-%s.csv"`, kind, a.today))
	// A byte-order mark makes spreadsheet programs read the file as UTF-8.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	out := csv.NewWriter(w)
	_ = out.Write(t.Columns)
	for _, row := range t.Rows {
		safe := make([]string, len(row))
		for i, cell := range row {
			safe[i] = csvSafe(cell)
		}
		_ = out.Write(safe)
	}
	out.Flush()
}

type auditView struct {
	domain.AuditLog
	ActorName string `json:"actorName,omitempty"`
}

// handleListAuditLogs returns the audit log, newest first.
func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	org, _, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	f := store.AuditFilter{Action: q.Get("action"), EntityType: q.Get("entityType"), EntityID: q.Get("entityId")}
	if raw := q.Get("actorId"); raw != "" {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest("actorId is not valid."))
			return
		}
		f.ActorID = &id
	}
	// Calendar dates are interpreted in the organization's time zone; `to` is inclusive.
	loc := org.Location()
	for name, target := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		raw := q.Get(name)
		if raw == "" {
			continue
		}
		d, err := domain.ParseDate(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest(name+" "+err.Error()+"."))
			return
		}
		at := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
		if name == "to" {
			at = at.AddDate(0, 0, 1)
		}
		*target = &at
	}
	page, size := httpx.Page(r)
	list, total, err := s.store.ListAuditLogs(r.Context(), orgID, f, page, size)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids := []bson.ObjectID{}
	for _, entry := range list {
		if entry.ActorID != nil && !slices.Contains(ids, *entry.ActorID) {
			ids = append(ids, *entry.ActorID)
		}
	}
	names, err := s.store.UserNames(r.Context(), orgID, ids)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]auditView, len(list))
	for i, entry := range list {
		out[i] = auditView{AuditLog: entry}
		if entry.ActorID != nil {
			out[i].ActorName = names[*entry.ActorID]
		}
	}
	httpx.JSON(w, 200, map[string]any{"data": out, "page": page, "pageSize": size, "total": total})
}
