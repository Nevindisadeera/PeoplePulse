package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9_-]{2,30}$`)

type orgView struct {
	*domain.Organization
	Settings map[string]any `json:"settings"`
}

func viewOrg(o *domain.Organization) orgView {
	chain := o.Settings.ApprovalChain
	if chain == nil {
		chain = []domain.ChainStep{}
	}
	template := o.Settings.ClearanceTemplate
	if template == nil {
		template = []domain.ClearanceTemplateItem{}
	}
	return orgView{Organization: o, Settings: map[string]any{
		"employmentTypes": o.EmploymentTypes(), "noticePeriodDays": o.NoticePeriodDays(),
		"separationReasons": o.SeparationReasons(), "approvalChain": chain, "clearanceTemplate": template,
	}}
}

// today returns the organization and the current calendar date in its time zone.
func (s *Server) today(ctx context.Context, orgID bson.ObjectID) (*domain.Organization, domain.Date, error) {
	org, err := s.store.OrganizationByID(ctx, orgID)
	if err != nil {
		return nil, domain.Date{}, err
	}
	return org, domain.Today(org.Location()), nil
}

func (s *Server) handleGetOrganization(w http.ResponseWriter, r *http.Request) {
	org, err := s.store.OrganizationByID(r.Context(), principal(r).User.OrganizationID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, viewOrg(org))
}

func (s *Server) handleUpdateOrganization(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		Name     *string `json:"name" validate:"omitempty,min=2,max=120"`
		TimeZone *string `json:"timeZone"`
		// Default notice period, used to propose a last working day.
		NoticePeriodDays *int `json:"noticePeriodDays" validate:"omitempty,min=0,max=365"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	set := bson.M{}
	if in.Name != nil {
		set["name"] = strings.TrimSpace(*in.Name)
	}
	if in.TimeZone != nil {
		if _, err := time.LoadLocation(*in.TimeZone); err != nil || *in.TimeZone == "" || *in.TimeZone == "Local" {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"timeZone": "must be a valid IANA time zone, for example Asia/Colombo"}))
			return
		}
		set["timeZone"] = *in.TimeZone
	}
	if in.NoticePeriodDays != nil {
		set["settings.noticePeriodDays"] = *in.NoticePeriodDays
	}
	if len(set) > 0 {
		changes := map[string]any{}
		for k, v := range set {
			changes[k] = v
		}
		if err := s.store.UpdateOrganization(r.Context(), orgID, set); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		s.audit(r, orgID, &p.User.ID, "organization.updated", "organization", orgID.Hex(), changes)
	}
	s.handleGetOrganization(w, r)
}

// handlePutEmploymentTypes replaces the list. A type that employees already
// use cannot be removed, only deactivated.
func (s *Server) handlePutEmploymentTypes(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		EmploymentTypes []domain.EmploymentType `json:"employmentTypes" validate:"required,min=1,max=30"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	seen, anyActive := map[string]bool{}, false
	for i := range in.EmploymentTypes {
		t := &in.EmploymentTypes[i]
		t.Code, t.Name = strings.ToLower(strings.TrimSpace(t.Code)), strings.TrimSpace(t.Name)
		switch {
		case !slugPattern.MatchString(t.Code):
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"employmentTypes": "code \"" + t.Code + "\" must be 2 to 30 lowercase letters, digits, hyphens or underscores"}))
			return
		case len(t.Name) < 2 || len(t.Name) > 60:
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"employmentTypes": "name for \"" + t.Code + "\" must be 2 to 60 characters"}))
			return
		case seen[t.Code]:
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"employmentTypes": "code \"" + t.Code + "\" appears more than once"}))
			return
		}
		seen[t.Code] = true
		anyActive = anyActive || t.Active
	}
	if !anyActive {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"employmentTypes": "at least one type must be active"}))
		return
	}
	org, err := s.store.OrganizationByID(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, old := range org.EmploymentTypes() {
		if seen[old.Code] {
			continue
		}
		n, err := s.store.CountEmployeesWithType(r.Context(), orgID, old.Code)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if n > 0 {
			httpx.Fail(w, r, httpx.Conflict("Employment type \""+old.Code+"\" is used by employee records. Deactivate it instead of removing it."))
			return
		}
	}
	if err := s.store.UpdateOrganization(r.Context(), orgID, bson.M{"settings.employmentTypes": in.EmploymentTypes}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "settings.employment_types.updated", "organization", orgID.Hex(),
		map[string]any{"from": org.EmploymentTypes(), "to": in.EmploymentTypes})
	httpx.JSON(w, 200, map[string]any{"employmentTypes": in.EmploymentTypes})
}

// handlePutSeparationReasons replaces the reason list. A reason already used
// by a separation cannot be removed, only deactivated, and keeps its category.
func (s *Server) handlePutSeparationReasons(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		SeparationReasons []domain.SeparationReason `json:"separationReasons" validate:"required,min=1,max=60"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bad := func(msg string) { httpx.Fail(w, r, httpx.Invalid(map[string]string{"separationReasons": msg})) }
	seen := map[string]domain.SeparationReason{}
	activeVoluntary := false
	for i := range in.SeparationReasons {
		x := &in.SeparationReasons[i]
		x.Code, x.Name = strings.ToLower(strings.TrimSpace(x.Code)), strings.TrimSpace(x.Name)
		_, dup := seen[x.Code]
		switch {
		case !slugPattern.MatchString(x.Code):
			bad(fmt.Sprintf("code %q must be 2 to 30 lowercase letters, digits, hyphens or underscores", x.Code))
			return
		case len(x.Name) < 2 || len(x.Name) > 80:
			bad(fmt.Sprintf("name for %q must be 2 to 80 characters", x.Code))
			return
		case x.Category != domain.CategoryVoluntary && x.Category != domain.CategoryInvoluntary && x.Category != domain.CategoryOther:
			bad(fmt.Sprintf("category for %q must be voluntary, involuntary or other", x.Code))
			return
		case dup:
			bad(fmt.Sprintf("code %q appears more than once", x.Code))
			return
		}
		seen[x.Code] = *x
		activeVoluntary = activeVoluntary || (x.Active && x.Category == domain.CategoryVoluntary)
	}
	if !activeVoluntary {
		bad("at least one active voluntary reason is required so resignations can be submitted")
		return
	}
	org, err := s.store.OrganizationByID(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, old := range org.SeparationReasons() {
		updated, kept := seen[old.Code]
		if kept && updated.Category == old.Category {
			continue
		}
		n, err := s.store.CountSeparationsWithReason(r.Context(), orgID, old.Code)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if n > 0 {
			httpx.Fail(w, r, httpx.Conflict(fmt.Sprintf("Reason %q is used by existing separations. It can be renamed or deactivated, but not removed or moved to another category.", old.Code)))
			return
		}
	}
	if err := s.store.UpdateOrganization(r.Context(), orgID, bson.M{"settings.separationReasons": in.SeparationReasons}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "settings.separation_reasons.updated", "organization", orgID.Hex(),
		map[string]any{"from": org.SeparationReasons(), "to": in.SeparationReasons})
	httpx.JSON(w, 200, map[string]any{"separationReasons": in.SeparationReasons})
}

// handlePutApprovalChain replaces the resignation approval chain (zero to
// three ordered steps). Requests already in progress keep the chain they started with.
func (s *Server) handlePutApprovalChain(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		ApprovalChain []struct {
			ApproverType string  `json:"approverType"`
			RoleID       *string `json:"roleId"`
			UserID       *string `json:"userId"`
		} `json:"approvalChain" validate:"required,max=3"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bad := func(step int, msg string) {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"approvalChain": fmt.Sprintf("step %d: %s", step, msg)}))
	}
	chain := []domain.ChainStep{}
	for i, c := range in.ApprovalChain {
		step := domain.ChainStep{ApproverType: c.ApproverType}
		switch c.ApproverType {
		case domain.ApproverLineManager:
		case domain.ApproverRole:
			id, err := bson.ObjectIDFromHex(deref(c.RoleID))
			if err == nil {
				_, err = s.store.RoleByID(r.Context(), orgID, id)
			}
			if err != nil {
				bad(i+1, "roleId is not a role in this organization")
				return
			}
			step.RoleID = &id
		case domain.ApproverUser:
			id, err := bson.ObjectIDFromHex(deref(c.UserID))
			var u *domain.User
			if err == nil {
				u, err = s.store.UserByID(r.Context(), orgID, id)
			}
			if err != nil || u.Status != domain.UserStatusActive {
				bad(i+1, "userId is not an active user in this organization")
				return
			}
			step.UserID = &id
		default:
			bad(i+1, "approverType must be line_manager, role or user")
			return
		}
		chain = append(chain, step)
	}
	org, err := s.store.OrganizationByID(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := s.store.UpdateOrganization(r.Context(), orgID, bson.M{"settings.approvalChain": chain}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "settings.approval_chain.updated", "organization", orgID.Hex(),
		map[string]any{"from": org.Settings.ApprovalChain, "to": chain})
	httpx.JSON(w, 200, map[string]any{"approvalChain": chain})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
