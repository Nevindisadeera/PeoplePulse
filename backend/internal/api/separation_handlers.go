package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

type employeeBrief struct {
	ID             bson.ObjectID `json:"id"`
	EmployeeNumber string        `json:"employeeNumber"`
	FullName       string        `json:"fullName"`
	DepartmentID   bson.ObjectID `json:"departmentId"`
	Status         string        `json:"status"`
}

type separationView struct {
	*domain.Separation
	Employee  *employeeBrief    `json:"employee,omitempty"`
	Clearance *clearanceSummary `json:"clearance,omitempty"`
	// CanAct is true when the caller is the approver of the pending step.
	CanAct bool `json:"canAct"`
}

func brief(e domain.Employee) *employeeBrief {
	return &employeeBrief{ID: e.ID, EmployeeNumber: e.EmployeeNumber, FullName: e.FullName, DepartmentID: e.DepartmentID, Status: e.Status}
}

func isOwnEmployee(p *Principal, employeeID bson.ObjectID) bool {
	return p.User.EmployeeID != nil && *p.User.EmployeeID == employeeID
}

// canActOn reports whether the caller may decide the pending step. Nobody may
// decide on their own separation.
func canActOn(p *Principal, sep *domain.Separation) bool {
	step := sep.PendingStep()
	return step != nil && p.Can(domain.PermSeparationApprove) &&
		step.AssignedTo(p.User.ID, p.Role.ID) && !isOwnEmployee(p, sep.EmployeeID)
}

// canSeeSeparation: HR sees everything; others see separations they
// submitted, their own, and those where they are an approver.
func canSeeSeparation(p *Principal, sep *domain.Separation) bool {
	if p.Can(domain.PermSeparationRead) || sep.SubmittedBy == p.User.ID || isOwnEmployee(p, sep.EmployeeID) {
		return true
	}
	if p.Can(domain.PermSeparationApprove) {
		for _, st := range sep.ApprovalSteps {
			if st.AssignedTo(p.User.ID, p.Role.ID) {
				return true
			}
		}
	}
	return false
}

func (s *Server) loadSeparation(r *http.Request) (*domain.Separation, error) {
	p := principal(r)
	id, err := pathID(r)
	if err != nil {
		return nil, httpx.NotFound("Separation")
	}
	sep, err := s.store.SeparationByID(r.Context(), p.User.OrganizationID, id)
	if err != nil {
		return nil, storeErr(err, "Separation")
	}
	if !canSeeSeparation(p, sep) {
		return nil, httpx.NotFound("Separation")
	}
	return sep, nil
}

func (s *Server) writeSeparation(w http.ResponseWriter, r *http.Request, status int, sep *domain.Separation) {
	p := principal(r)
	view := separationView{Separation: sep, CanAct: canActOn(p, sep)}
	if e, err := s.store.EmployeeByID(r.Context(), sep.OrganizationID, sep.EmployeeID); err == nil {
		view.Employee = brief(*e)
	}
	if c, err := s.store.ClearanceBySeparation(r.Context(), sep.OrganizationID, sep.ID); err == nil {
		view.Clearance = summarize(c)
	}
	httpx.JSON(w, status, view)
}

func transitionErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidTransition):
		return httpx.Conflict("This action is not possible in the separation's current status.")
	case errors.Is(err, domain.ErrNotYetLastDay):
		return httpx.Conflict("The separation can be completed on or after the last working day.")
	}
	return err
}

func (s *Server) handleListSeparations(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	q := r.URL.Query()
	f := store.SeparationFilter{Status: q.Get("status")}
	if f.Status != "" && !slices.Contains([]string{domain.SeparationPending, domain.SeparationApproved, domain.SeparationRejected, domain.SeparationWithdrawn, domain.SeparationCompleted}, f.Status) {
		httpx.Fail(w, r, httpx.BadRequest("status is not a valid separation status."))
		return
	}
	if raw := q.Get("employeeId"); raw != "" {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest("employeeId is not valid."))
			return
		}
		f.EmployeeID = &id
	}
	if !p.Can(domain.PermSeparationRead) {
		v := &store.SeparationVisibility{UserID: p.User.ID, EmployeeID: p.User.EmployeeID}
		if p.Can(domain.PermSeparationApprove) {
			v.RoleID = &p.Role.ID
		}
		f.Visibility = v
	}
	if q.Get("awaitingMe") == "true" && p.Can(domain.PermSeparationApprove) {
		f.AwaitingUser, f.AwaitingRole = &p.User.ID, &p.Role.ID
	}
	page, size := httpx.Page(r)
	list, total, err := s.store.ListSeparations(r.Context(), orgID, f, page, size)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids := make([]bson.ObjectID, len(list))
	for i := range list {
		ids[i] = list[i].EmployeeID
	}
	emps, err := s.store.EmployeesByIDs(r.Context(), orgID, ids)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]separationView, len(list))
	for i := range list {
		out[i] = separationView{Separation: &list[i], CanAct: canActOn(p, &list[i])}
		if e, ok := emps[list[i].EmployeeID]; ok {
			out[i].Employee = brief(e)
		}
	}
	httpx.JSON(w, 200, map[string]any{"data": out, "page": page, "pageSize": size, "total": total})
}

func (s *Server) handleGetSeparation(w http.ResponseWriter, r *http.Request) {
	sep, err := s.loadSeparation(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.writeSeparation(w, r, 200, sep)
}

// handleCreateSeparation covers three cases: an employee submitting their own
// resignation, HR submitting a resignation on someone's behalf (both go
// through the approval chain), and HR recording another type of separation
// (approved immediately).
func (s *Server) handleCreateSeparation(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		EmployeeID              *string `json:"employeeId"`
		Type                    string  `json:"type"`
		ReasonCode              string  `json:"reasonCode" validate:"required"`
		ReasonDetails           string  `json:"reasonDetails" validate:"max=2000"`
		SubmissionDate          *string `json:"submissionDate"`
		RequestedLastWorkingDay *string `json:"requestedLastWorkingDay"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Type == "" {
		in.Type = domain.SeparationTypeResignation
	}
	if !slices.Contains(domain.SeparationTypes, in.Type) {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"type": "must be one of: " + strings.Join(domain.SeparationTypes, ", ")}))
		return
	}

	manage := p.Can(domain.PermSeparationManage)
	var employeeID bson.ObjectID
	switch {
	case manage && in.EmployeeID != nil && *in.EmployeeID != "":
		id, err := bson.ObjectIDFromHex(*in.EmployeeID)
		if err != nil {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"employeeId": "is not an employee in this organization"}))
			return
		}
		employeeID = id
	case p.User.EmployeeID != nil && (in.EmployeeID == nil || *in.EmployeeID == "" || *in.EmployeeID == p.User.EmployeeID.Hex()):
		employeeID = *p.User.EmployeeID
	case manage:
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"employeeId": "is required"}))
		return
	case p.User.EmployeeID == nil:
		httpx.Fail(w, r, httpx.Conflict("Your account is not linked to an employee record. Ask HR to link it or to submit the resignation for you."))
		return
	default:
		httpx.Fail(w, r, httpx.Forbidden())
		return
	}
	self := isOwnEmployee(p, employeeID)
	if !manage && in.Type != domain.SeparationTypeResignation {
		httpx.Fail(w, r, httpx.Forbidden())
		return
	}

	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	reason, ok := org.Reason(in.ReasonCode)
	if !ok {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"reasonCode": "is not an active separation reason in this organization"}))
		return
	}
	isResignation := in.Type == domain.SeparationTypeResignation
	if isResignation != (reason.Category == domain.CategoryVoluntary) {
		msg := "must be a voluntary reason for a resignation"
		if !isResignation {
			msg = "must be an involuntary or other reason for this separation type"
		}
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"reasonCode": msg}))
		return
	}
	if reason.Category == domain.CategoryInvoluntary && !p.Can(domain.PermSeparationRecordInv) {
		httpx.Fail(w, r, httpx.Forbidden())
		return
	}

	emp, err := s.store.EmployeeByID(r.Context(), orgID, employeeID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			err = httpx.Invalid(map[string]string{"employeeId": "is not an employee in this organization"})
		}
		httpx.Fail(w, r, err)
		return
	}
	if emp.Status != domain.EmployeeActive {
		httpx.Fail(w, r, httpx.Conflict("This employee is already serving notice or has left."))
		return
	}

	submission := today
	if in.SubmissionDate != nil && manage && !self {
		if submission, err = domain.ParseDate(*in.SubmissionDate); err != nil {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"submissionDate": err.Error()}))
			return
		}
		if submission.After(today) || submission.Before(emp.JoinDate) {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"submissionDate": "must be between the employee's join date and today"}))
			return
		}
	}
	lastDay := submission.AddDays(org.NoticePeriodDays())
	if in.RequestedLastWorkingDay != nil && *in.RequestedLastWorkingDay != "" {
		if lastDay, err = domain.ParseDate(*in.RequestedLastWorkingDay); err != nil {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"requestedLastWorkingDay": err.Error()}))
			return
		}
		if lastDay.Before(submission) {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"requestedLastWorkingDay": "cannot be before the submission date"}))
			return
		}
		if lastDay.After(submission.AddYears(2)) {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"requestedLastWorkingDay": "cannot be more than two years after the submission date"}))
			return
		}
	}

	now := time.Now().UTC()
	sep := &domain.Separation{
		ID: bson.NewObjectID(), OrganizationID: orgID, EmployeeID: emp.ID, Type: in.Type,
		ReasonCode: reason.Code, Category: reason.Category, ReasonDetails: strings.TrimSpace(in.ReasonDetails),
		SubmittedBy: p.User.ID, SubmissionDate: submission, RequestedLastWorkingDay: lastDay, LastWorkingDay: lastDay,
		ApprovalSteps: []domain.ApprovalStep{}, StatusHistory: []domain.StatusChange{},
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if isResignation {
		if sep.ApprovalSteps, err = s.resolveChain(r.Context(), org, emp); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	sep.Start(p.User.ID, now)

	var onApproved func(ctx context.Context) error
	if sep.Status == domain.SeparationApproved {
		if onApproved, err = s.approvedSideEffects(r.Context(), p.User.ID, org, emp, sep, now); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	err = s.store.Atomic(r.Context(), func(ctx context.Context) error {
		if err := s.store.InsertSeparation(ctx, sep); err != nil {
			return err
		}
		if onApproved != nil {
			if err := onApproved(ctx); err != nil {
				return err
			}
		}
		return s.notifySeparationStarted(ctx, p.User.ID, sep)
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Conflict("This employee already has a separation in progress.")
		}
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "separation.submitted", "separation", sep.ID.Hex(), map[string]any{
		"employeeId": emp.ID.Hex(), "type": sep.Type, "reasonCode": sep.ReasonCode, "status": sep.Status,
		"lastWorkingDay": lastDay.String(), "onBehalf": !self,
	})
	s.writeSeparation(w, r, 201, sep)
}

// resolveChain copies the organization's approval chain onto a request and
// resolves who approves each step for this employee. A step whose approver
// cannot be resolved, or would be the employee themselves, goes to HR.
func (s *Server) resolveChain(ctx context.Context, org *domain.Organization, emp *domain.Employee) ([]domain.ApprovalStep, error) {
	var fallback *bson.ObjectID
	hrFallback := func() (*bson.ObjectID, error) {
		if fallback != nil {
			return fallback, nil
		}
		role, err := s.store.RoleByName(ctx, org.ID, "HR Manager")
		if errors.Is(err, store.ErrNotFound) {
			role, err = s.store.LockedRole(ctx, org.ID)
		}
		if err != nil {
			return nil, err
		}
		fallback = &role.ID
		return fallback, nil
	}
	activeUser := func(id bson.ObjectID) bool {
		u, err := s.store.UserByID(ctx, org.ID, id)
		return err == nil && u.Status == domain.UserStatusActive && (u.EmployeeID == nil || *u.EmployeeID != emp.ID)
	}

	steps := []domain.ApprovalStep{}
	for i, c := range org.Settings.ApprovalChain {
		step := domain.ApprovalStep{Order: i + 1, ApproverType: c.ApproverType, Decision: domain.DecisionPending}
		switch c.ApproverType {
		case domain.ApproverLineManager:
			if emp.ManagerID != nil {
				if u, err := s.store.ActiveUserByEmployeeID(ctx, org.ID, *emp.ManagerID); err == nil {
					step.UserID = &u.ID
				} else if !errors.Is(err, store.ErrNotFound) {
					return nil, err
				}
			}
		case domain.ApproverUser:
			if c.UserID != nil && activeUser(*c.UserID) {
				step.UserID = c.UserID
			}
		case domain.ApproverRole:
			if c.RoleID != nil {
				if _, err := s.store.RoleByID(ctx, org.ID, *c.RoleID); err == nil {
					step.RoleID = c.RoleID
				}
			}
		}
		if step.UserID == nil && step.RoleID == nil {
			id, err := hrFallback()
			if err != nil {
				return nil, err
			}
			step.RoleID, step.Fallback = id, true
		}
		steps = append(steps, step)
	}
	return steps, nil
}

type decisionInput struct {
	Comment string `json:"comment" validate:"max=1000"`
	Version int    `json:"version" validate:"required"`
}

func (s *Server) handleApproveSeparation(w http.ResponseWriter, r *http.Request) {
	s.decide(w, r, true)
}

func (s *Server) handleRejectSeparation(w http.ResponseWriter, r *http.Request) {
	s.decide(w, r, false)
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	p := principal(r)
	var in decisionInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in.Comment = strings.TrimSpace(in.Comment)
	if !approve && in.Comment == "" {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"comment": "is required when rejecting"}))
		return
	}
	sep, err := s.loadSeparation(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	step := sep.PendingStep()
	if step == nil {
		httpx.Fail(w, r, transitionErr(domain.ErrInvalidTransition))
		return
	}
	if isOwnEmployee(p, sep.EmployeeID) {
		httpx.Fail(w, r, &httpx.Error{Status: 403, Code: "FORBIDDEN", Message: "You cannot decide on your own resignation."})
		return
	}
	if !step.AssignedTo(p.User.ID, p.Role.ID) {
		httpx.Fail(w, r, &httpx.Error{Status: 403, Code: "FORBIDDEN", Message: "This approval step is assigned to someone else."})
		return
	}
	stepOrder, now, final := step.Order, time.Now().UTC(), false
	if approve {
		final, err = sep.Approve(p.User.ID, in.Comment, now)
	} else {
		err = sep.Reject(p.User.ID, in.Comment, now)
	}
	if err != nil {
		httpx.Fail(w, r, transitionErr(err))
		return
	}
	var onApproved func(ctx context.Context) error
	if final {
		org, err := s.store.OrganizationByID(r.Context(), sep.OrganizationID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		emp, err := s.store.EmployeeByID(r.Context(), sep.OrganizationID, sep.EmployeeID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if onApproved, err = s.approvedSideEffects(r.Context(), p.User.ID, org, emp, sep, now); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	err = s.store.Atomic(r.Context(), func(ctx context.Context) error {
		if err := s.store.SaveSeparation(ctx, sep, in.Version); err != nil {
			return err
		}
		if onApproved != nil {
			if err := onApproved(ctx); err != nil {
				return err
			}
		}
		return s.notifyDecision(ctx, p.User.ID, sep)
	})
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Separation"))
		return
	}
	action := "separation.step_approved"
	switch {
	case !approve:
		action = "separation.rejected"
	case final:
		action = "separation.approved"
	}
	s.audit(r, sep.OrganizationID, &p.User.ID, action, "separation", sep.ID.Hex(), map[string]any{"step": stepOrder, "comment": in.Comment})
	sep.Version++
	s.writeSeparation(w, r, 200, sep)
}

// handleWithdrawSeparation: the employee, the submitter or HR can withdraw a
// pending request. Once approved, only HR can cancel it, with a reason.
func (s *Server) handleWithdrawSeparation(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Note    string `json:"note" validate:"max=1000"`
		Version int    `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	sep, err := s.loadSeparation(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	manage := p.Can(domain.PermSeparationManage)
	wasApproved := sep.Status == domain.SeparationApproved
	switch {
	case wasApproved && !manage:
		httpx.Fail(w, r, &httpx.Error{Status: 403, Code: "FORBIDDEN", Message: "This resignation has been approved. Ask HR to cancel it."})
		return
	case wasApproved && in.Note == "":
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"note": "is required when cancelling an approved separation"}))
		return
	case !manage && !isOwnEmployee(p, sep.EmployeeID) && sep.SubmittedBy != p.User.ID:
		httpx.Fail(w, r, httpx.Forbidden())
		return
	}
	// Whoever had been asked to approve is told that the request is gone.
	wasAwaiting, err := s.pendingApprovers(r.Context(), sep)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := sep.Withdraw(p.User.ID, in.Note, time.Now().UTC()); err != nil {
		httpx.Fail(w, r, transitionErr(err))
		return
	}
	err = s.store.Atomic(r.Context(), func(ctx context.Context) error {
		if err := s.store.SaveSeparation(ctx, sep, in.Version); err != nil {
			return err
		}
		if wasApproved {
			if err := s.store.CancelClearance(ctx, sep.OrganizationID, sep.ID); err != nil {
				return err
			}
			if err := s.store.PatchEmployee(ctx, sep.OrganizationID, sep.EmployeeID, bson.M{"status": domain.EmployeeActive}); err != nil {
				return err
			}
		}
		return s.notifyWithdrawn(ctx, p.User.ID, sep, wasAwaiting)
	})
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Separation"))
		return
	}
	s.audit(r, sep.OrganizationID, &p.User.ID, "separation.withdrawn", "separation", sep.ID.Hex(), map[string]any{"note": in.Note, "wasApproved": wasApproved})
	sep.Version++
	s.writeSeparation(w, r, 200, sep)
}

func (s *Server) handleSetLastWorkingDay(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		LastWorkingDay string `json:"lastWorkingDay" validate:"required"`
		Reason         string `json:"reason" validate:"required,max=500"`
		Version        int    `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sep, err := s.loadSeparation(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !sep.IsOpen() {
		httpx.Fail(w, r, transitionErr(domain.ErrInvalidTransition))
		return
	}
	day, err := domain.ParseDate(in.LastWorkingDay)
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"lastWorkingDay": err.Error()}))
		return
	}
	if day.Before(sep.SubmissionDate) || day.After(sep.SubmissionDate.AddYears(2)) {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"lastWorkingDay": "must be on or after the submission date and within two years of it"}))
		return
	}
	if day.Equal(sep.LastWorkingDay) {
		s.writeSeparation(w, r, 200, sep)
		return
	}
	old := sep.LastWorkingDay
	sep.LastWorkingDay = day
	sep.StatusHistory = append(sep.StatusHistory, domain.StatusChange{
		Status: sep.Status, At: time.Now().UTC(), By: &p.User.ID,
		Note: "Last working day changed from " + old.String() + " to " + day.String() + ": " + strings.TrimSpace(in.Reason),
	})
	err = s.store.Atomic(r.Context(), func(ctx context.Context) error {
		if err := s.store.SaveSeparation(ctx, sep, in.Version); err != nil {
			return err
		}
		// Open clearance items that were due on the old last working day move with it.
		if err := s.store.ShiftClearanceDueDates(ctx, sep.OrganizationID, sep.ID, old, day); err != nil {
			return err
		}
		return s.notifyLastWorkingDayChanged(ctx, p.User.ID, sep, old)
	})
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Separation"))
		return
	}
	s.audit(r, sep.OrganizationID, &p.User.ID, "separation.last_working_day_changed", "separation", sep.ID.Hex(),
		map[string]any{"from": old.String(), "to": day.String(), "reason": in.Reason})
	sep.Version++
	s.writeSeparation(w, r, 200, sep)
}

// handleCompleteSeparation closes the employment period: the employee gets an
// end date and the separated status, and the current department assignment is closed.
func (s *Server) handleCompleteSeparation(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Version int `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sep, err := s.loadSeparation(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, today, err := s.today(r.Context(), sep.OrganizationID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	emp, err := s.store.EmployeeByID(r.Context(), sep.OrganizationID, sep.EmployeeID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := sep.Complete(p.User.ID, emp, today, time.Now().UTC()); err != nil {
		httpx.Fail(w, r, transitionErr(err))
		return
	}
	// Clearance must be closed, or overridden by HR, before the employee leaves the books.
	clearance, err := s.store.ClearanceBySeparation(r.Context(), sep.OrganizationID, sep.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		httpx.Fail(w, r, err)
		return
	}
	if err == nil && !clearance.ReadyForCompletion() {
		httpx.Fail(w, r, httpx.Conflict(fmt.Sprintf("%d clearance item(s) are still open. Close them, or record a clearance override, before completing the separation.", clearance.OpenItems())))
		return
	}
	end := sep.LastWorkingDay
	set := bson.M{"status": domain.EmployeeSeparated, "endDate": end}
	if n := len(emp.DepartmentHistory); n > 0 {
		history := slices.Clone(emp.DepartmentHistory)
		history[n-1].To = &end
		set["departmentHistory"] = history
	}
	err = s.store.Atomic(r.Context(), func(ctx context.Context) error {
		if err := s.store.SaveSeparation(ctx, sep, in.Version); err != nil {
			return err
		}
		return s.store.PatchEmployee(ctx, sep.OrganizationID, emp.ID, set)
	})
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Separation"))
		return
	}
	s.audit(r, sep.OrganizationID, &p.User.ID, "separation.completed", "separation", sep.ID.Hex(),
		map[string]any{"employeeId": emp.ID.Hex(), "endDate": end.String(), "tenureDays": *sep.TenureDaysAtExit})
	sep.Version++
	s.writeSeparation(w, r, 200, sep)
}
