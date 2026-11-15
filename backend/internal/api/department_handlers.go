package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

var deptCodePattern = regexp.MustCompile(`^[A-Z0-9-]{2,20}$`)

const maxDepartmentDepth = 5

type departmentView struct {
	domain.Department
	Headcount int64 `json:"headcount"`
}

// storeErr maps data-layer errors to API errors for a named kind of record.
func storeErr(err error, what string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return httpx.NotFound(what)
	case errors.Is(err, store.ErrVersion):
		return httpx.Conflict("This record was changed by someone else. Reload it and try again.")
	}
	return err
}

func (s *Server) handleListDepartments(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	status := r.URL.Query().Get("status")
	if status != "" && status != domain.DepartmentActive && status != domain.DepartmentArchived {
		httpx.Fail(w, r, httpx.BadRequest("status must be active or archived."))
		return
	}
	depts, err := s.store.ListDepartments(r.Context(), orgID, status)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, err := s.store.HeadcountByDepartment(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]departmentView, len(depts))
	for i, d := range depts {
		out[i] = departmentView{Department: d, Headcount: counts[d.ID]}
	}
	httpx.JSON(w, 200, map[string]any{"data": out})
}

func (s *Server) handleGetDepartment(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := s.store.DepartmentByID(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Department"))
		return
	}
	n, err := s.store.CountCurrentEmployeesInDepartment(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, departmentView{Department: *d, Headcount: n})
}

func (s *Server) handleCreateDepartment(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		Name           string  `json:"name" validate:"required,min=2,max=100"`
		Code           string  `json:"code" validate:"required"`
		ParentID       *string `json:"parentId"`
		HeadEmployeeID *string `json:"headEmployeeId"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	code, err := cleanDeptCode(in.Code)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	d := &domain.Department{
		ID: bson.NewObjectID(), OrganizationID: orgID, Name: strings.TrimSpace(in.Name), Code: code,
		Status: domain.DepartmentActive, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if in.ParentID != nil && *in.ParentID != "" {
		parent, err := s.checkParent(r.Context(), orgID, d.ID, *in.ParentID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		d.ParentID = &parent
	}
	if in.HeadEmployeeID != nil && *in.HeadEmployeeID != "" {
		head, err := s.checkCurrentEmployee(r.Context(), orgID, *in.HeadEmployeeID, "headEmployeeId")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		d.HeadEmployeeID = &head
	}
	if err := s.store.InsertDepartment(r.Context(), d); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Invalid(map[string]string{"code": "is already used by another department"})
		}
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "department.created", "department", d.ID.Hex(), map[string]any{"name": d.Name, "code": d.Code})
	httpx.JSON(w, 201, departmentView{Department: *d})
}

func (s *Server) handleUpdateDepartment(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// For parentId and headEmployeeId an empty string clears the value.
	var in struct {
		Name           *string `json:"name" validate:"omitempty,min=2,max=100"`
		Code           *string `json:"code"`
		ParentID       *string `json:"parentId"`
		HeadEmployeeID *string `json:"headEmployeeId"`
		Version        int     `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := s.store.DepartmentByID(r.Context(), orgID, id); err != nil {
		httpx.Fail(w, r, storeErr(err, "Department"))
		return
	}
	set, changes, unset := bson.M{}, map[string]any{}, []string{}
	if in.Name != nil {
		set["name"] = strings.TrimSpace(*in.Name)
		changes["name"] = set["name"]
	}
	if in.Code != nil {
		code, err := cleanDeptCode(*in.Code)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		set["code"] = code
		changes["code"] = code
	}
	if in.ParentID != nil {
		if *in.ParentID == "" {
			unset = append(unset, "parentId")
		} else {
			parent, err := s.checkParent(r.Context(), orgID, id, *in.ParentID)
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			set["parentId"] = parent
		}
		changes["parentId"] = *in.ParentID
	}
	if in.HeadEmployeeID != nil {
		if *in.HeadEmployeeID == "" {
			unset = append(unset, "headEmployeeId")
		} else {
			head, err := s.checkCurrentEmployee(r.Context(), orgID, *in.HeadEmployeeID, "headEmployeeId")
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			set["headEmployeeId"] = head
		}
		changes["headEmployeeId"] = *in.HeadEmployeeID
	}
	if len(changes) > 0 {
		if err := s.store.UpdateDepartment(r.Context(), orgID, id, in.Version, set, unset...); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				err = httpx.Invalid(map[string]string{"code": "is already used by another department"})
			}
			httpx.Fail(w, r, storeErr(err, "Department"))
			return
		}
		s.audit(r, orgID, &p.User.ID, "department.updated", "department", id.Hex(), changes)
	}
	s.handleGetDepartment(w, r)
}

func (s *Server) handleArchiveDepartment(w http.ResponseWriter, r *http.Request) {
	s.setDepartmentStatus(w, r, domain.DepartmentArchived)
}

func (s *Server) handleRestoreDepartment(w http.ResponseWriter, r *http.Request) {
	s.setDepartmentStatus(w, r, domain.DepartmentActive)
}

func (s *Server) setDepartmentStatus(w http.ResponseWriter, r *http.Request, status string) {
	p := principal(r)
	orgID := p.User.OrganizationID
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := s.store.DepartmentByID(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Department"))
		return
	}
	if d.Status != status {
		if status == domain.DepartmentArchived {
			n, err := s.store.CountCurrentEmployeesInDepartment(r.Context(), orgID, id)
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if n > 0 {
				httpx.Fail(w, r, httpx.Conflict("This department still has employees. Transfer them before archiving it."))
				return
			}
			children, err := s.store.CountActiveChildDepartments(r.Context(), orgID, id)
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if children > 0 {
				httpx.Fail(w, r, httpx.Conflict("This department has active sub-departments. Archive or move them first."))
				return
			}
		}
		if err := s.store.UpdateDepartment(r.Context(), orgID, id, d.Version, bson.M{"status": status}); err != nil {
			httpx.Fail(w, r, storeErr(err, "Department"))
			return
		}
		s.audit(r, orgID, &p.User.ID, "department."+status, "department", id.Hex(), nil)
	}
	s.handleGetDepartment(w, r)
}

func cleanDeptCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if !deptCodePattern.MatchString(code) {
		return "", httpx.Invalid(map[string]string{"code": "must be 2 to 20 letters, digits or hyphens"})
	}
	return code, nil
}

// checkParent verifies the parent exists, is active, and that making it the
// parent of self creates neither a cycle nor a hierarchy that is too deep.
func (s *Server) checkParent(ctx context.Context, orgID, self bson.ObjectID, parentHex string) (bson.ObjectID, error) {
	bad := func(msg string) (bson.ObjectID, error) {
		return bson.ObjectID{}, httpx.Invalid(map[string]string{"parentId": msg})
	}
	parentID, err := bson.ObjectIDFromHex(parentHex)
	if err != nil {
		return bad("is not a department in this organization")
	}
	cursor := parentID
	for depth := 1; ; depth++ {
		if cursor == self {
			return bad("would make the department its own ancestor")
		}
		if depth >= maxDepartmentDepth {
			return bad("would make the hierarchy deeper than 5 levels")
		}
		d, err := s.store.DepartmentByID(ctx, orgID, cursor)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return bad("is not a department in this organization")
			}
			return bson.ObjectID{}, err
		}
		if depth == 1 && d.Status != domain.DepartmentActive {
			return bad("is an archived department")
		}
		if d.ParentID == nil {
			return parentID, nil
		}
		cursor = *d.ParentID
	}
}

// checkCurrentEmployee verifies the ID names an employee who has not separated.
func (s *Server) checkCurrentEmployee(ctx context.Context, orgID bson.ObjectID, idHex, field string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(idHex)
	if err == nil {
		var e *domain.Employee
		if e, err = s.store.EmployeeByID(ctx, orgID, id); err == nil && e.Status == domain.EmployeeSeparated {
			return id, httpx.Invalid(map[string]string{field: "is no longer employed"})
		}
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || id.IsZero() {
			return id, httpx.Invalid(map[string]string{field: "is not an employee in this organization"})
		}
		return id, err
	}
	return id, nil
}
