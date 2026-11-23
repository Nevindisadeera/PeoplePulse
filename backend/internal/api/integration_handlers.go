package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/metrics"
)

// HeaderIntegrationKey carries the service key of the automation tool.
const HeaderIntegrationKey = "X-Integration-Key"

// integrationAuth admits only the automation tool. The key gives access to
// these read-only endpoints and nothing else: it cannot approve, edit or read
// any other record. With no key configured the endpoints do not exist.
func (s *Server) integrationAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.IntegrationKey == "" {
			httpx.Fail(w, r, httpx.NotFound("Route"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderIntegrationKey)), []byte(s.cfg.IntegrationKey)) != 1 {
			httpx.Fail(w, r, httpx.Unauthorized("A valid integration key is required."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type recipient struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

// recipientsFor turns user IDs into active users' contact details, in a stable order.
func (s *Server) recipientsFor(ctx context.Context, orgID bson.ObjectID, ids []bson.ObjectID) ([]recipient, error) {
	unique := []bson.ObjectID{}
	for _, id := range ids {
		if !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}
	users, err := s.store.ActiveUsersByIDs(ctx, orgID, unique)
	if err != nil {
		return nil, err
	}
	out := make([]recipient, len(users))
	for i, u := range users {
		out[i] = recipient{UserID: u.ID.Hex(), Email: u.Email, Name: u.FullName}
	}
	slices.SortFunc(out, func(a, b recipient) int { return cmpString(a.Email, b.Email) })
	return out, nil
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

type clearanceReminderItem struct {
	EmployeeName   string      `json:"employeeName"`
	EmployeeNumber string      `json:"employeeNumber"`
	Category       string      `json:"category"`
	Title          string      `json:"title"`
	Status         string      `json:"status"`
	DueDate        domain.Date `json:"dueDate"`
	Overdue        bool        `json:"overdue"`
	DaysUntilDue   int         `json:"daysUntilDue"` // negative when overdue
	Link           string      `json:"link"`
}

type clearanceReminder struct {
	recipient
	Items []clearanceReminderItem `json:"items"`
}

// handleClearanceReminders returns, for every organization, the open
// clearance items that are overdue or due within the organization's reminder
// window, grouped by the person who must act. The API decides what is due;
// the automation tool only sends what it is given.
func (s *Server) handleClearanceReminders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgs, err := s.store.ListOrganizations(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type orgBlock struct {
		OrganizationID string              `json:"organizationId"`
		Name           string              `json:"organizationName"`
		Today          domain.Date         `json:"today"`
		WindowDays     int                 `json:"windowDays"`
		Reminders      []clearanceReminder `json:"reminders"`
	}
	out := []orgBlock{}
	for i := range orgs {
		org := &orgs[i]
		today := domain.Today(org.Location())
		horizon := today.AddDays(org.ClearanceReminderDays())
		clearances, err := s.store.OpenClearances(ctx, org.ID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		views, err := s.clearanceViews(ctx, org.ID, clearances)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		byUser := map[bson.ObjectID][]clearanceReminderItem{}
		for _, v := range views {
			if v.Employee == nil {
				continue
			}
			for _, item := range v.Items {
				if !item.Open() || item.DueDate.After(horizon) {
					continue
				}
				users, err := s.assignedUsers(ctx, org.ID, item.UserID, item.RoleID)
				if err != nil {
					httpx.Fail(w, r, err)
					return
				}
				entry := clearanceReminderItem{
					EmployeeName: v.Employee.FullName, EmployeeNumber: v.Employee.EmployeeNumber, Category: item.Category, Title: item.Title,
					Status: item.Status, DueDate: item.DueDate, Overdue: item.DueDate.Before(today),
					DaysUntilDue: int(item.DueDate.Sub(today.Time).Hours() / 24),
					Link:         s.cfg.AppBaseURL + "/clearance/" + v.ID.Hex(),
				}
				for _, id := range users {
					byUser[id] = append(byUser[id], entry)
				}
			}
		}
		ids := make([]bson.ObjectID, 0, len(byUser))
		for id := range byUser {
			ids = append(ids, id)
		}
		people, err := s.recipientsFor(ctx, org.ID, ids)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		block := orgBlock{OrganizationID: org.ID.Hex(), Name: org.Name, Today: today, WindowDays: org.ClearanceReminderDays(), Reminders: []clearanceReminder{}}
		for _, person := range people {
			id, _ := bson.ObjectIDFromHex(person.UserID)
			items := byUser[id]
			slices.SortStableFunc(items, func(a, b clearanceReminderItem) int { return a.DueDate.Compare(b.DueDate.Time) })
			block.Reminders = append(block.Reminders, clearanceReminder{recipient: person, Items: items})
		}
		if len(block.Reminders) > 0 {
			out = append(out, block)
		}
	}
	httpx.JSON(w, 200, map[string]any{"generatedAt": time.Now().UTC(), "organizations": out})
}

type lastDayItem struct {
	EmployeeName       string      `json:"employeeName"`
	EmployeeNumber     string      `json:"employeeNumber"`
	SeparationType     string      `json:"separationType"`
	LastWorkingDay     domain.Date `json:"lastWorkingDay"`
	DaysLeft           int         `json:"daysLeft"` // 0 on the last working day, negative after it
	OpenClearanceItems int         `json:"openClearanceItems"`
	Link               string      `json:"link"`
}

// handleLastDayReminders returns approved separations whose last working day
// falls within the organization's reminder window (or has passed without the
// separation being completed), with the people to tell: HR and the line manager.
func (s *Server) handleLastDayReminders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgs, err := s.store.ListOrganizations(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type reminder struct {
		recipient
		Items []lastDayItem `json:"items"`
	}
	type orgBlock struct {
		OrganizationID string      `json:"organizationId"`
		Name           string      `json:"organizationName"`
		Today          domain.Date `json:"today"`
		WindowDays     int         `json:"windowDays"`
		Reminders      []reminder  `json:"reminders"`
	}
	out := []orgBlock{}
	for i := range orgs {
		org := &orgs[i]
		today := domain.Today(org.Location())
		seps, err := s.store.ApprovedSeparationsEndingBy(ctx, org.ID, today.AddDays(org.LastDayReminderDays()))
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if len(seps) == 0 {
			continue
		}
		hr, err := s.usersWithPermission(ctx, org.ID, domain.PermSeparationManage)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		empIDs, sepIDs := make([]bson.ObjectID, len(seps)), make([]bson.ObjectID, len(seps))
		for j := range seps {
			empIDs[j], sepIDs[j] = seps[j].EmployeeID, seps[j].ID
		}
		emps, err := s.store.EmployeesByIDs(ctx, org.ID, empIDs)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		clearances, err := s.store.ClearancesBySeparations(ctx, org.ID, sepIDs)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		byUser := map[bson.ObjectID][]lastDayItem{}
		for _, sep := range seps {
			emp, ok := emps[sep.EmployeeID]
			if !ok {
				continue
			}
			item := lastDayItem{EmployeeName: emp.FullName, EmployeeNumber: emp.EmployeeNumber, SeparationType: sep.Type, LastWorkingDay: sep.LastWorkingDay,
				DaysLeft: int(sep.LastWorkingDay.Sub(today.Time).Hours() / 24), Link: s.cfg.AppBaseURL + "/separations/" + sep.ID.Hex()}
			if c, ok := clearances[sep.ID]; ok {
				item.OpenClearanceItems = c.OpenItems()
			}
			to := slices.Clone(hr)
			if emp.ManagerID != nil {
				manager, err := s.employeeUser(ctx, org.ID, *emp.ManagerID)
				if err != nil {
					httpx.Fail(w, r, err)
					return
				}
				to = append(to, manager...)
			}
			for _, id := range to {
				if !slices.ContainsFunc(byUser[id], func(x lastDayItem) bool { return x.Link == item.Link }) {
					byUser[id] = append(byUser[id], item)
				}
			}
		}
		ids := make([]bson.ObjectID, 0, len(byUser))
		for id := range byUser {
			ids = append(ids, id)
		}
		people, err := s.recipientsFor(ctx, org.ID, ids)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		block := orgBlock{OrganizationID: org.ID.Hex(), Name: org.Name, Today: today, WindowDays: org.LastDayReminderDays(), Reminders: []reminder{}}
		for _, person := range people {
			id, _ := bson.ObjectIDFromHex(person.UserID)
			block.Reminders = append(block.Reminders, reminder{recipient: person, Items: byUser[id]})
		}
		if len(block.Reminders) > 0 {
			out = append(out, block)
		}
	}
	httpx.JSON(w, 200, map[string]any{"generatedAt": time.Now().UTC(), "organizations": out})
}

// handleMonthlyReportData returns each organization's figures for one
// calendar month (?month=YYYY-MM, default: the month before the current one).
// Every number comes from the metrics package; the automation tool formats
// and sends the report and calculates nothing.
func (s *Server) handleMonthlyReportData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var requested *domain.Date
	if raw := r.URL.Query().Get("month"); raw != "" {
		d, err := domain.ParseDate(raw + "-01")
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest("month must be in YYYY-MM format."))
			return
		}
		requested = &d
	}
	orgs, err := s.store.ListOrganizations(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type orgReport struct {
		OrganizationID     string           `json:"organizationId"`
		Name               string           `json:"organizationName"`
		Month              string           `json:"month"`
		From               domain.Date      `json:"from"`
		To                 domain.Date      `json:"to"`
		OpeningHeadcount   int              `json:"openingHeadcount"`
		Joiners            int              `json:"joiners"`
		Leavers            int              `json:"leavers"`
		ClosingHeadcount   int              `json:"closingHeadcount"`
		Turnover           metrics.Turnover `json:"turnover"`
		Rolling12Months    metrics.Turnover `json:"rolling12Months"`
		AwaitingApproval   int64            `json:"separationsAwaitingApproval"`
		ServingNotice      int64            `json:"employeesServingNotice"`
		OpenClearanceItems int              `json:"openClearanceItems"`
		OverdueClearance   int              `json:"overdueClearanceItems"`
		Warnings           []string         `json:"warnings"`
		Recipients         []recipient      `json:"recipients"`
		Link               string           `json:"link"`
	}
	out := []orgReport{}
	for i := range orgs {
		org := &orgs[i]
		today := domain.Today(org.Location())
		from := metrics.MonthsBack(today, 1)
		if requested != nil {
			from = *requested
		}
		if from.After(today) {
			httpx.Fail(w, r, httpx.BadRequest("month cannot be in the future."))
			return
		}
		to := domain.Date{Time: from.AddDate(0, 1, -1)}
		ds, err := s.loadDataset(ctx, org)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		a := &analysis{org: org, today: today, from: from, to: to, ds: ds, small: org.SmallGroupThreshold(), warnings: []string{}}
		if org.Settings.AnalyticsStartDate == nil {
			a.warnings = append(a.warnings, warnNoAnalyticsStart)
		} else if a.beforeStart(from) {
			a.warnings = append(a.warnings, warnBeforeStart)
		}
		months := a.guardMonths(ds.Monthly(from, to, nil, a.small))
		if len(months) == 0 {
			continue
		}
		m := months[0]
		rollingFrom := to.AddYears(-1).AddDays(1)
		report := orgReport{OrganizationID: org.ID.Hex(), Name: org.Name, Month: m.Period, From: from, To: m.To,
			OpeningHeadcount: m.Opening, Joiners: m.Joiners, Leavers: m.Leavers, ClosingHeadcount: m.Closing, Turnover: m.Turnover,
			Rolling12Months: a.guardTurnover(ds.Turnover(rollingFrom, m.To, nil, a.small), rollingFrom),
			Warnings:        a.warnings, Link: s.cfg.AppBaseURL + "/"}
		if m.Partial {
			report.Warnings = append(report.Warnings, "month_not_finished")
		}
		if report.AwaitingApproval, err = s.store.CountSeparationsByStatus(ctx, org.ID, domain.SeparationPending); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if report.ServingNotice, err = s.store.CountSeparationsByStatus(ctx, org.ID, domain.SeparationApproved); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		clearances, err := s.store.OpenClearances(ctx, org.ID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		for _, c := range clearances {
			for _, item := range c.Items {
				if item.Open() {
					report.OpenClearanceItems++
					if item.DueDate.Before(today) {
						report.OverdueClearance++
					}
				}
			}
		}
		// The report goes to everyone who may export reports.
		ids, err := s.usersWithPermission(ctx, org.ID, domain.PermReportExport)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if report.Recipients, err = s.recipientsFor(ctx, org.ID, ids); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out = append(out, report)
	}
	httpx.JSON(w, 200, map[string]any{"generatedAt": time.Now().UTC(), "organizations": out})
}
