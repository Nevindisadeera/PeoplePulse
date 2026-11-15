package api

import (
	"context"
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
	return orgView{Organization: o, Settings: map[string]any{"employmentTypes": o.EmploymentTypes()}}
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
