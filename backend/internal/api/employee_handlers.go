package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

// employeeScope reports which employees the caller may see: everyone, or only
// those in the departments the caller heads (an empty list means nobody).
func (s *Server) employeeScope(r *http.Request) (all bool, deptIDs []bson.ObjectID, err error) {
	p := principal(r)
	if p.Can(domain.PermEmployeeRead) {
		return true, nil, nil
	}
	if p.User.EmployeeID == nil {
		return false, []bson.ObjectID{}, nil
	}
	ids, err := s.store.DepartmentIDsHeadedBy(r.Context(), p.User.OrganizationID, *p.User.EmployeeID)
	if ids == nil {
		ids = []bson.ObjectID{}
	}
	return false, ids, err
}

// loadEmployee fetches an employee the caller is allowed to see. Out-of-scope
// records look the same as missing ones.
func (s *Server) loadEmployee(r *http.Request) (*domain.Employee, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, httpx.NotFound("Employee")
	}
	e, err := s.store.EmployeeByID(r.Context(), principal(r).User.OrganizationID, id)
	if err != nil {
		return nil, storeErr(err, "Employee")
	}
	all, depts, err := s.employeeScope(r)
	if err != nil {
		return nil, err
	}
	if !all && !slices.Contains(depts, e.DepartmentID) {
		return nil, httpx.NotFound("Employee")
	}
	return e, nil
}

func (s *Server) handleListEmployees(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	q := r.URL.Query()
	f := store.EmployeeFilter{Query: strings.TrimSpace(q.Get("q")), Status: q.Get("status")}
	if f.Status != "" && !slices.Contains([]string{domain.EmployeeActive, domain.EmployeeServingNotice, domain.EmployeeSeparated}, f.Status) {
		httpx.Fail(w, r, httpx.BadRequest("status must be active, serving_notice or separated."))
		return
	}
	all, scope, err := s.employeeScope(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !all {
		f.DepartmentIDs = scope
	}
	if raw := q.Get("departmentId"); raw != "" {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest("departmentId is not valid."))
			return
		}
		if all || slices.Contains(scope, id) {
			f.DepartmentIDs = []bson.ObjectID{id}
		} else {
			f.DepartmentIDs = []bson.ObjectID{}
		}
	}
	page, size := httpx.Page(r)
	list, total, err := s.store.ListEmployees(r.Context(), orgID, f, page, size)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": list, "page": page, "pageSize": size, "total": total})
}

func (s *Server) handleGetEmployee(w http.ResponseWriter, r *http.Request) {
	e, err := s.loadEmployee(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, e)
}

func (s *Server) handleCreateEmployee(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		EmployeeNumber string  `json:"employeeNumber" validate:"required,max=30"`
		FullName       string  `json:"fullName" validate:"required,min=2,max=120"`
		WorkEmail      string  `json:"workEmail" validate:"omitempty,email,max=254"`
		Phone          string  `json:"phone" validate:"max=30"`
		DepartmentID   string  `json:"departmentId" validate:"required"`
		JobTitle       string  `json:"jobTitle" validate:"max=120"`
		EmploymentType string  `json:"employmentType" validate:"required"`
		ManagerID      *string `json:"managerId"`
		JoinDate       string  `json:"joinDate" validate:"required"`
		RehireOf       *string `json:"rehireOf"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	join, err := checkJoinDate(in.JoinDate, today)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := checkEmploymentType(org, in.EmploymentType); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	deptID, err := s.checkActiveDepartment(r.Context(), orgID, in.DepartmentID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	e := &domain.Employee{
		ID: bson.NewObjectID(), OrganizationID: orgID, EmployeeNumber: strings.TrimSpace(in.EmployeeNumber),
		FullName: strings.TrimSpace(in.FullName), WorkEmail: strings.ToLower(strings.TrimSpace(in.WorkEmail)),
		Phone: strings.TrimSpace(in.Phone), DepartmentID: deptID, JobTitle: strings.TrimSpace(in.JobTitle),
		DepartmentHistory: []domain.DepartmentAssignment{{DepartmentID: deptID, From: join}},
		EmploymentType:    in.EmploymentType, JoinDate: join, Status: domain.EmployeeActive,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if in.ManagerID != nil && *in.ManagerID != "" {
		mgr, err := s.checkManager(r.Context(), orgID, e.ID, *in.ManagerID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		e.ManagerID = &mgr
	}
	if in.RehireOf != nil && *in.RehireOf != "" {
		prevID, err := bson.ObjectIDFromHex(*in.RehireOf)
		var prev *domain.Employee
		if err == nil {
			prev, err = s.store.EmployeeByID(r.Context(), orgID, prevID)
		}
		switch {
		case err != nil:
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"rehireOf": "is not an employee in this organization"}))
			return
		case prev.Status != domain.EmployeeSeparated || prev.EndDate == nil || !join.After(*prev.EndDate):
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"rehireOf": "must be a separated employee whose end date is before the new join date"}))
			return
		}
		e.RehireOf = &prevID
	}
	if err := s.store.InsertEmployee(r.Context(), e); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Invalid(map[string]string{"employeeNumber": "is already used by another employee"})
		}
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "employee.created", "employee", e.ID.Hex(),
		map[string]any{"employeeNumber": e.EmployeeNumber, "departmentId": deptID.Hex(), "joinDate": join.String()})
	httpx.JSON(w, 201, e)
}

func (s *Server) handleUpdateEmployee(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	// managerId: an empty string clears the manager.
	var in struct {
		FullName       *string `json:"fullName" validate:"omitempty,min=2,max=120"`
		WorkEmail      *string `json:"workEmail" validate:"omitempty,email,max=254"`
		Phone          *string `json:"phone" validate:"omitempty,max=30"`
		JobTitle       *string `json:"jobTitle" validate:"omitempty,max=120"`
		EmploymentType *string `json:"employmentType"`
		ManagerID      *string `json:"managerId"`
		JoinDate       *string `json:"joinDate"`
		Version        int     `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := s.loadEmployee(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	set, changes, unset := bson.M{}, map[string]any{}, []string{}
	change := func(field string, from, to any) {
		set[field] = to
		changes[field] = map[string]any{"from": from, "to": to}
	}
	if in.FullName != nil && strings.TrimSpace(*in.FullName) != e.FullName {
		change("fullName", e.FullName, strings.TrimSpace(*in.FullName))
	}
	if in.WorkEmail != nil && strings.ToLower(strings.TrimSpace(*in.WorkEmail)) != e.WorkEmail {
		change("workEmail", e.WorkEmail, strings.ToLower(strings.TrimSpace(*in.WorkEmail)))
	}
	if in.Phone != nil && strings.TrimSpace(*in.Phone) != e.Phone {
		change("phone", e.Phone, strings.TrimSpace(*in.Phone))
	}
	if in.JobTitle != nil && strings.TrimSpace(*in.JobTitle) != e.JobTitle {
		change("jobTitle", e.JobTitle, strings.TrimSpace(*in.JobTitle))
	}
	if in.EmploymentType != nil && *in.EmploymentType != e.EmploymentType {
		if err := checkEmploymentType(org, *in.EmploymentType); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		change("employmentType", e.EmploymentType, *in.EmploymentType)
	}
	if in.ManagerID != nil {
		if *in.ManagerID == "" {
			if e.ManagerID != nil {
				unset = append(unset, "managerId")
				changes["managerId"] = map[string]any{"from": e.ManagerID.Hex(), "to": nil}
			}
		} else {
			mgr, err := s.checkManager(r.Context(), orgID, e.ID, *in.ManagerID)
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if e.ManagerID == nil || *e.ManagerID != mgr {
				set["managerId"] = mgr
				changes["managerId"] = map[string]any{"to": mgr.Hex()}
			}
		}
	}
	if in.JoinDate != nil {
		join, err := checkJoinDate(*in.JoinDate, today)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if !join.Equal(e.JoinDate) {
			// A join date correction is only safe while nothing else depends on it.
			if len(e.DepartmentHistory) != 1 || e.EndDate != nil {
				httpx.Fail(w, r, httpx.Conflict("The join date cannot be changed after a transfer or separation has been recorded."))
				return
			}
			set["joinDate"] = join
			set["departmentHistory.0.from"] = join
			changes["joinDate"] = map[string]any{"from": e.JoinDate.String(), "to": join.String()}
		}
	}
	if len(changes) > 0 {
		if err := s.store.UpdateEmployee(r.Context(), orgID, e.ID, in.Version, set, unset...); err != nil {
			httpx.Fail(w, r, storeErr(err, "Employee"))
			return
		}
		s.audit(r, orgID, &p.User.ID, "employee.updated", "employee", e.ID.Hex(), changes)
	}
	s.handleGetEmployee(w, r)
}

// handleTransferEmployee moves an employee to another department from an
// effective date, keeping the dated history.
func (s *Server) handleTransferEmployee(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		DepartmentID  string `json:"departmentId" validate:"required"`
		EffectiveDate string `json:"effectiveDate" validate:"required"`
		Version       int    `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := s.loadEmployee(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if e.Status == domain.EmployeeSeparated {
		httpx.Fail(w, r, httpx.Conflict("This employee has left and cannot be transferred."))
		return
	}
	_, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	effective, err := domain.ParseDate(in.EffectiveDate)
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"effectiveDate": err.Error()}))
		return
	}
	if effective.After(today) {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"effectiveDate": "cannot be in the future"}))
		return
	}
	deptID, err := s.checkActiveDepartment(r.Context(), orgID, in.DepartmentID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	history, err := domain.ApplyTransfer(e.DepartmentHistory, deptID, effective)
	switch {
	case errors.Is(err, domain.ErrSameDepartment):
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"departmentId": "is the employee's current department"}))
		return
	case errors.Is(err, domain.ErrTransferTooEarly):
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"effectiveDate": "must be after " + e.DepartmentHistory[len(e.DepartmentHistory)-1].From.String() + ", when the current assignment started"}))
		return
	case err != nil:
		httpx.Fail(w, r, err)
		return
	}
	err = s.store.UpdateEmployee(r.Context(), orgID, e.ID, in.Version, bson.M{"departmentId": deptID, "departmentHistory": history})
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Employee"))
		return
	}
	s.audit(r, orgID, &p.User.ID, "employee.transferred", "employee", e.ID.Hex(),
		map[string]any{"from": e.DepartmentID.Hex(), "to": deptID.Hex(), "effectiveDate": effective.String()})
	updated, err := s.store.EmployeeByID(r.Context(), orgID, e.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, updated)
}

type timelineEvent struct {
	Date           domain.Date `json:"date"`
	Type           string      `json:"type"` // joined | transferred | separated
	DepartmentID   string      `json:"departmentId,omitempty"`
	DepartmentName string      `json:"departmentName,omitempty"`
	FromDepartment string      `json:"fromDepartmentName,omitempty"`
}

func (s *Server) handleEmployeeTimeline(w http.ResponseWriter, r *http.Request) {
	e, err := s.loadEmployee(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	depts, err := s.store.ListDepartments(r.Context(), e.OrganizationID, "")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	names := map[bson.ObjectID]string{}
	for _, d := range depts {
		names[d.ID] = d.Name
	}
	events := []timelineEvent{}
	for i, a := range e.DepartmentHistory {
		ev := timelineEvent{Date: a.From, Type: "transferred", DepartmentID: a.DepartmentID.Hex(), DepartmentName: names[a.DepartmentID]}
		if i == 0 {
			ev.Type, ev.Date = "joined", e.JoinDate
		} else {
			ev.FromDepartment = names[e.DepartmentHistory[i-1].DepartmentID]
		}
		events = append(events, ev)
	}
	if e.EndDate != nil {
		events = append(events, timelineEvent{Date: *e.EndDate, Type: "separated"})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Date.Before(events[j].Date) })
	httpx.JSON(w, 200, map[string]any{"data": events})
}

// ---- shared checks ----

func checkJoinDate(raw string, today domain.Date) (domain.Date, error) {
	d, err := domain.ParseDate(raw)
	if err != nil {
		return d, httpx.Invalid(map[string]string{"joinDate": err.Error()})
	}
	if d.After(today.AddYears(1)) {
		return d, httpx.Invalid(map[string]string{"joinDate": "cannot be more than one year in the future"})
	}
	if d.Before(domain.NewDate(1950, 1, 1)) {
		return d, httpx.Invalid(map[string]string{"joinDate": "is too far in the past"})
	}
	return d, nil
}

func checkEmploymentType(org *domain.Organization, code string) error {
	for _, t := range org.EmploymentTypes() {
		if t.Code == code && t.Active {
			return nil
		}
	}
	return httpx.Invalid(map[string]string{"employmentType": "is not an active employment type in this organization"})
}

func (s *Server) checkActiveDepartment(ctx context.Context, orgID bson.ObjectID, idHex string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(idHex)
	if err != nil {
		return id, httpx.Invalid(map[string]string{"departmentId": "is not a department in this organization"})
	}
	d, err := s.store.DepartmentByID(ctx, orgID, id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return id, httpx.Invalid(map[string]string{"departmentId": "is not a department in this organization"})
	case err != nil:
		return id, err
	case d.Status != domain.DepartmentActive:
		return id, httpx.Invalid(map[string]string{"departmentId": "is an archived department"})
	}
	return id, nil
}

// checkManager verifies the manager is a current employee, is not the employee
// themselves, and does not report (directly or indirectly) to the employee.
func (s *Server) checkManager(ctx context.Context, orgID, self bson.ObjectID, idHex string) (bson.ObjectID, error) {
	mgrID, err := s.checkCurrentEmployee(ctx, orgID, idHex, "managerId")
	if err != nil {
		return mgrID, err
	}
	cursor := mgrID
	for range 50 {
		if cursor == self {
			return mgrID, httpx.Invalid(map[string]string{"managerId": "cannot be the employee or someone who reports to them"})
		}
		m, err := s.store.EmployeeByID(ctx, orgID, cursor)
		if err != nil || m.ManagerID == nil {
			break
		}
		cursor = *m.ManagerID
	}
	return mgrID, nil
}
