package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

func (s *Server) handleListPermissions(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, 200, map[string]any{"data": domain.Catalogue})
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.store.ListRoles(r.Context(), principal(r).User.OrganizationID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": roles})
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Name        string   `json:"name" validate:"required,min=2,max=60"`
		Description string   `json:"description" validate:"max=300"`
		Permissions []string `json:"permissions" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	perms, err := cleanPermissions(in.Permissions)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	role := &domain.Role{
		ID: bson.NewObjectID(), OrganizationID: p.User.OrganizationID, Name: strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description), Permissions: perms, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.InsertRole(r.Context(), role); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Invalid(map[string]string{"name": "is already used by another role"})
		}
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, role.OrganizationID, &p.User.ID, "role.created", "role", role.ID.Hex(), map[string]any{"name": role.Name, "permissions": perms})
	httpx.JSON(w, 201, role)
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Name        *string   `json:"name" validate:"omitempty,min=2,max=60"`
		Description *string   `json:"description" validate:"omitempty,max=300"`
		Permissions *[]string `json:"permissions"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	role, err := s.store.RoleByID(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, notFoundOr(err, "Role"))
		return
	}
	if role.Locked && (in.Permissions != nil || in.Name != nil) {
		httpx.Fail(w, r, httpx.Conflict("The Organization Admin role cannot be renamed or have its permissions changed."))
		return
	}
	set, changes := bson.M{}, map[string]any{}
	if in.Name != nil {
		set["name"] = strings.TrimSpace(*in.Name)
		changes["name"] = map[string]any{"from": role.Name, "to": set["name"]}
	}
	if in.Description != nil {
		set["description"] = strings.TrimSpace(*in.Description)
	}
	if in.Permissions != nil {
		perms, err := cleanPermissions(*in.Permissions)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		set["permissions"] = perms
		changes["permissions"] = map[string]any{"from": role.Permissions, "to": perms}
	}
	if len(set) > 0 {
		if err := s.store.UpdateRole(r.Context(), orgID, id, set); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				err = httpx.Invalid(map[string]string{"name": "is already used by another role"})
			}
			httpx.Fail(w, r, notFoundOr(err, "Role"))
			return
		}
		s.audit(r, orgID, &p.User.ID, "role.updated", "role", id.Hex(), changes)
	}
	updated, err := s.store.RoleByID(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, updated)
}

// cleanPermissions de-duplicates the list and rejects keys outside the catalogue.
func cleanPermissions(in []string) ([]string, error) {
	seen, out := map[string]bool{}, []string{}
	for _, k := range in {
		if !domain.IsValidPermission(k) {
			return nil, httpx.Invalid(map[string]string{"permissions": "contains an unknown permission: " + k})
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, nil
}
