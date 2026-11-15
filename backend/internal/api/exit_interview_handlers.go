package api

import (
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

type interviewView struct {
	*domain.ExitInterview
	Employee *employeeBrief `json:"employee,omitempty"`
}

func isHR(p *Principal) bool {
	return p.Can(domain.PermExitInterviewRead) || p.Can(domain.PermExitInterviewManage)
}

// loadInterview fetches an interview for HR, or for the departing employee
// when self-service is on. selfService reports which of the two applies.
func (s *Server) loadInterview(r *http.Request) (iv *domain.ExitInterview, selfService bool, err error) {
	p := principal(r)
	id, err := pathID(r)
	if err != nil {
		return nil, false, httpx.NotFound("Exit interview")
	}
	iv, err = s.store.ExitInterviewByID(r.Context(), p.User.OrganizationID, id)
	if err != nil {
		return nil, false, storeErr(err, "Exit interview")
	}
	if isHR(p) {
		return iv, false, nil
	}
	if isOwnEmployee(p, iv.EmployeeID) {
		org, err := s.store.OrganizationByID(r.Context(), iv.OrganizationID)
		if err != nil {
			return nil, false, err
		}
		if org.InterviewTemplate().EmployeeSelfService {
			return iv, true, nil
		}
	}
	return nil, false, httpx.NotFound("Exit interview")
}

// writeInterview hides HR's notes and the interviewer from the employee's own view.
func (s *Server) writeInterview(w http.ResponseWriter, r *http.Request, status int, iv *domain.ExitInterview, selfService bool) {
	out := *iv
	if selfService {
		out.Notes, out.InterviewerID = "", nil
	}
	view := interviewView{ExitInterview: &out}
	if e, err := s.store.EmployeeByID(r.Context(), iv.OrganizationID, iv.EmployeeID); err == nil {
		view.Employee = brief(*e)
	}
	httpx.JSON(w, status, view)
}

func answerProblems(problems map[string]string) error {
	fields := map[string]string{}
	for id, msg := range problems {
		fields["answers."+id] = msg
	}
	return httpx.Invalid(fields)
}

// handleCreateExitInterview starts an interview for an approved or completed
// separation, copying the current question template onto it.
func (s *Server) handleCreateExitInterview(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		InterviewerID *string    `json:"interviewerId"`
		ScheduledAt   *time.Time `json:"scheduledAt"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sepID, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, httpx.NotFound("Separation"))
		return
	}
	sep, err := s.store.SeparationByID(r.Context(), orgID, sepID)
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Separation"))
		return
	}
	if sep.Status != domain.SeparationApproved && sep.Status != domain.SeparationCompleted {
		httpx.Fail(w, r, httpx.Conflict("An exit interview can be created once the separation is approved."))
		return
	}
	org, err := s.store.OrganizationByID(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	template := org.InterviewTemplate()
	questions := []domain.InterviewQuestion{}
	for _, q := range template.Questions {
		if q.Active {
			questions = append(questions, q)
		}
	}
	if !template.Enabled || len(questions) == 0 {
		httpx.Fail(w, r, httpx.Conflict("Exit interviews are switched off or have no active questions. Update the exit interview settings first."))
		return
	}
	now := time.Now().UTC()
	iv := &domain.ExitInterview{
		ID: bson.NewObjectID(), OrganizationID: orgID, SeparationID: sep.ID, EmployeeID: sep.EmployeeID,
		Status: domain.InterviewDraft, Questions: questions, Answers: []domain.InterviewAnswer{},
		ScheduledAt: in.ScheduledAt, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if in.InterviewerID != nil && *in.InterviewerID != "" {
		if iv.InterviewerID, err = s.checkInterviewer(r, *in.InterviewerID); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if err := s.store.InsertExitInterview(r.Context(), iv); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Conflict("This separation already has an exit interview.")
		}
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "exit_interview.created", "exit_interview", iv.ID.Hex(), map[string]any{"separationId": sep.ID.Hex()})
	s.writeInterview(w, r, 201, iv, false)
}

func (s *Server) checkInterviewer(r *http.Request, idHex string) (*bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(idHex)
	var u *domain.User
	if err == nil {
		u, err = s.store.UserByID(r.Context(), principal(r).User.OrganizationID, id)
	}
	if err != nil || u.Status != domain.UserStatusActive {
		return nil, httpx.Invalid(map[string]string{"interviewerId": "is not an active user in this organization"})
	}
	return &id, nil
}

// handleGetSeparationInterview finds the interview for a separation, which is
// how a departing employee reaches their own form.
func (s *Server) handleGetSeparationInterview(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	sepID, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, httpx.NotFound("Exit interview"))
		return
	}
	iv, err := s.store.ExitInterviewBySeparation(r.Context(), p.User.OrganizationID, sepID)
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Exit interview"))
		return
	}
	self := !isHR(p)
	if self {
		org, err := s.store.OrganizationByID(r.Context(), iv.OrganizationID)
		if err != nil || !isOwnEmployee(p, iv.EmployeeID) || !org.InterviewTemplate().EmployeeSelfService {
			httpx.Fail(w, r, httpx.NotFound("Exit interview"))
			return
		}
	}
	s.writeInterview(w, r, 200, iv, self)
}

func (s *Server) handleListExitInterviews(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	status := r.URL.Query().Get("status")
	if status != "" && status != domain.InterviewDraft && status != domain.InterviewSubmitted {
		httpx.Fail(w, r, httpx.BadRequest("status must be draft or submitted."))
		return
	}
	page, size := httpx.Page(r)
	list, total, err := s.store.ListExitInterviews(r.Context(), orgID, status, page, size)
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
	out := make([]interviewView, len(list))
	for i := range list {
		out[i] = interviewView{ExitInterview: &list[i]}
		if e, ok := emps[list[i].EmployeeID]; ok {
			out[i].Employee = brief(e)
		}
	}
	httpx.JSON(w, 200, map[string]any{"data": out, "page": page, "pageSize": size, "total": total})
}

func (s *Server) handleGetExitInterview(w http.ResponseWriter, r *http.Request) {
	iv, self, err := s.loadInterview(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.writeInterview(w, r, 200, iv, self)
}

// handleUpdateExitInterview saves a draft. The employee may change answers
// only. A submitted interview is locked: changing it needs the amend
// permission and a reason, and is audited.
func (s *Server) handleUpdateExitInterview(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Answers       *[]domain.InterviewAnswer `json:"answers"`
		Notes         *string                   `json:"notes" validate:"omitempty,max=4000"`
		InterviewerID *string                   `json:"interviewerId"`
		ScheduledAt   *time.Time                `json:"scheduledAt"`
		AmendReason   string                    `json:"amendReason" validate:"max=500"`
		Version       int                       `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	iv, self, err := s.loadInterview(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	submitted := iv.Status == domain.InterviewSubmitted
	switch {
	case self && (submitted || in.Notes != nil || in.InterviewerID != nil || in.ScheduledAt != nil):
		httpx.Fail(w, r, &httpx.Error{Status: 403, Code: "FORBIDDEN", Message: "You can only change your answers, and only before the form is submitted."})
		return
	case !self && !p.Can(domain.PermExitInterviewManage):
		httpx.Fail(w, r, httpx.Forbidden())
		return
	case submitted && !p.Can(domain.PermExitInterviewAmend):
		httpx.Fail(w, r, &httpx.Error{Status: 403, Code: "FORBIDDEN", Message: "This interview has been submitted. Only a user with permission to amend can change it."})
		return
	case submitted && strings.TrimSpace(in.AmendReason) == "":
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"amendReason": "is required to change a submitted interview"}))
		return
	}
	changed := []string{}
	if in.Answers != nil {
		clean, problems := domain.CleanAnswers(iv.Questions, *in.Answers, submitted)
		if len(problems) > 0 {
			httpx.Fail(w, r, answerProblems(problems))
			return
		}
		iv.Answers = clean
		changed = append(changed, "answers")
	}
	if in.Notes != nil {
		iv.Notes = strings.TrimSpace(*in.Notes)
		changed = append(changed, "notes")
	}
	if in.InterviewerID != nil {
		iv.InterviewerID = nil
		if *in.InterviewerID != "" {
			if iv.InterviewerID, err = s.checkInterviewer(r, *in.InterviewerID); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		}
		changed = append(changed, "interviewerId")
	}
	if in.ScheduledAt != nil {
		iv.ScheduledAt = in.ScheduledAt
		changed = append(changed, "scheduledAt")
	}
	if len(changed) > 0 {
		if err := s.store.SaveExitInterview(r.Context(), iv, in.Version); err != nil {
			httpx.Fail(w, r, storeErr(err, "Exit interview"))
			return
		}
		iv.Version++
		action, details := "exit_interview.updated", map[string]any{"fields": changed, "byEmployee": self}
		if submitted {
			action, details["reason"] = "exit_interview.amended", strings.TrimSpace(in.AmendReason)
		}
		// Answer content is deliberately kept out of the audit log.
		s.audit(r, iv.OrganizationID, &p.User.ID, action, "exit_interview", iv.ID.Hex(), details)
	}
	s.writeInterview(w, r, 200, iv, self)
}

// handleSubmitExitInterview locks the interview after checking that every
// required question is answered.
func (s *Server) handleSubmitExitInterview(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Version int `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	iv, self, err := s.loadInterview(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !self && !p.Can(domain.PermExitInterviewManage) {
		httpx.Fail(w, r, httpx.Forbidden())
		return
	}
	if iv.Status == domain.InterviewSubmitted {
		httpx.Fail(w, r, httpx.Conflict("This interview has already been submitted."))
		return
	}
	clean, problems := domain.CleanAnswers(iv.Questions, iv.Answers, true)
	if len(problems) > 0 {
		httpx.Fail(w, r, answerProblems(problems))
		return
	}
	now := time.Now().UTC()
	iv.Answers, iv.Status, iv.SubmittedAt, iv.SubmittedBy = clean, domain.InterviewSubmitted, &now, &p.User.ID
	if err := s.store.SaveExitInterview(r.Context(), iv, in.Version); err != nil {
		httpx.Fail(w, r, storeErr(err, "Exit interview"))
		return
	}
	iv.Version++
	s.audit(r, iv.OrganizationID, &p.User.ID, "exit_interview.submitted", "exit_interview", iv.ID.Hex(), map[string]any{"byEmployee": self})
	s.writeInterview(w, r, 200, iv, self)
}

// handleExitFeedback returns aggregated answers for interviews submitted in a
// period. Individual responses and free text are never included.
func (s *Server) handleExitFeedback(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	from, to := today.AddYears(-1).AddDays(1), today
	q := r.URL.Query()
	if raw := q.Get("from"); raw != "" {
		if from, err = domain.ParseDate(raw); err != nil {
			httpx.Fail(w, r, httpx.BadRequest("from "+err.Error()+"."))
			return
		}
	}
	if raw := q.Get("to"); raw != "" {
		if to, err = domain.ParseDate(raw); err != nil {
			httpx.Fail(w, r, httpx.BadRequest("to "+err.Error()+"."))
			return
		}
	}
	if from.After(to) || to.After(from.AddYears(5)) {
		httpx.Fail(w, r, httpx.BadRequest("from must not be after to, and the range can be at most five years."))
		return
	}
	// Interpret the calendar dates in the organization's time zone.
	loc := org.Location()
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	list, err := s.store.SubmittedExitInterviews(r.Context(), orgID, start, end)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summary := domain.AggregateFeedback(list, org.InterviewTemplate().MinAggregateResponses)
	httpx.JSON(w, 200, map[string]any{"from": from, "to": to, "summary": summary})
}

func (s *Server) handleGetInterviewTemplate(w http.ResponseWriter, r *http.Request) {
	org, err := s.store.OrganizationByID(r.Context(), principal(r).User.OrganizationID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, org.InterviewTemplate())
}

// handlePutInterviewTemplate replaces the question template. Interviews
// already created keep the questions they were created with. Send a
// question's existing id to keep it; a question without an id is new.
func (s *Server) handlePutInterviewTemplate(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		Enabled               bool `json:"enabled"`
		EmployeeSelfService   bool `json:"employeeSelfService"`
		MinAggregateResponses *int `json:"minAggregateResponses" validate:"omitempty,min=1,max=50"`
		Questions             []struct {
			ID       string   `json:"id"`
			Text     string   `json:"text"`
			Type     string   `json:"type"`
			Options  []string `json:"options"`
			Required bool     `json:"required"`
			Active   *bool    `json:"active"`
		} `json:"questions" validate:"max=40"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	org, err := s.store.OrganizationByID(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	existing := map[string]domain.InterviewQuestion{}
	for _, q := range org.InterviewTemplate().Questions {
		existing[q.ID] = q
	}
	t := domain.ExitInterviewTemplate{Enabled: in.Enabled, EmployeeSelfService: in.EmployeeSelfService,
		MinAggregateResponses: domain.DefaultMinAggregateResponses, Questions: []domain.InterviewQuestion{}}
	if in.MinAggregateResponses != nil {
		t.MinAggregateResponses = *in.MinAggregateResponses
	}
	seen, anyActive := map[string]bool{}, false
	for i, q := range in.Questions {
		bad := func(msg string) {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"questions": fmt.Sprintf("question %d: %s", i+1, msg)}))
		}
		out := domain.InterviewQuestion{ID: q.ID, Text: strings.TrimSpace(q.Text), Type: q.Type, Required: q.Required, Active: q.Active == nil || *q.Active}
		old, known := existing[q.ID]
		switch {
		case q.ID == "":
			out.ID = bson.NewObjectID().Hex()
		case !known:
			bad("id does not match an existing question; leave it empty for a new question")
			return
		case seen[q.ID]:
			bad("appears more than once")
			return
		case old.Type != q.Type:
			bad("the type of an existing question cannot be changed; deactivate it and add a new one")
			return
		}
		seen[out.ID] = true
		if n := len(out.Text); n < 3 || n > 300 {
			bad("text must be 3 to 300 characters")
			return
		}
		switch q.Type {
		case domain.QuestionRating, domain.QuestionText:
		case domain.QuestionChoice:
			for _, o := range q.Options {
				if o = strings.TrimSpace(o); o != "" && len(o) <= 80 && !slices.Contains(out.Options, o) {
					out.Options = append(out.Options, o)
				}
			}
			if len(out.Options) < 2 || len(out.Options) > 10 || len(out.Options) != len(q.Options) {
				bad("a choice question needs 2 to 10 distinct, non-empty options of at most 80 characters")
				return
			}
		default:
			bad("type must be rating, choice or text")
			return
		}
		anyActive = anyActive || out.Active
		t.Questions = append(t.Questions, out)
	}
	if t.Enabled && !anyActive {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"questions": "at least one active question is needed while exit interviews are enabled"}))
		return
	}
	if err := s.store.UpdateOrganization(r.Context(), orgID, bson.M{"settings.exitInterview": t}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "settings.exit_interview.updated", "organization", orgID.Hex(),
		map[string]any{"enabled": t.Enabled, "employeeSelfService": t.EmployeeSelfService, "questions": len(t.Questions)})
	httpx.JSON(w, 200, t)
}
