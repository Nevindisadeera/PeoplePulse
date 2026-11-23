package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/auth"
	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	page, size := httpx.Page(r)
	users, total, err := s.store.ListUsers(r.Context(), p.User.OrganizationID, page, size)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": users, "page": page, "pageSize": size, "total": total})
}

// handleInviteUser creates an invited user and returns a single-use invite
// token. Email delivery through n8n is added in Phase 6; until then the admin
// passes the link on.
func (s *Server) handleInviteUser(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Email    string `json:"email" validate:"required,email,max=254"`
		FullName string `json:"fullName" validate:"required,min=2,max=120"`
		RoleID   string `json:"roleId" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	orgID := p.User.OrganizationID
	roleID, err := bson.ObjectIDFromHex(in.RoleID)
	if err == nil {
		_, err = s.store.RoleByID(r.Context(), orgID, roleID)
	}
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"roleId": "is not a role in this organization"}))
		return
	}
	token, hash, err := auth.NewOpaqueToken()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	expires := now.Add(s.cfg.InviteTTL)
	user := &domain.User{
		ID: bson.NewObjectID(), OrganizationID: orgID, Email: strings.ToLower(strings.TrimSpace(in.Email)),
		FullName: strings.TrimSpace(in.FullName), RoleID: roleID, Status: domain.UserStatusInvited,
		InviteTokenHash: hash, InviteExpiresAt: &expires, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.InsertUser(r.Context(), user); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Invalid(map[string]string{"email": "is already in use"})
		}
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "user.invited", "user", user.ID.Hex(), map[string]any{"email": user.Email, "roleId": roleID.Hex()})
	s.sendLink(r, user, domain.EventUserInvited, "You have been invited to PeoplePulse",
		"An account has been created for you. Choose a password to activate it. The link works once and expires in 72 hours.", "/accept-invite?token="+token)
	httpx.JSON(w, 201, map[string]any{"user": user, "inviteToken": token, "inviteExpiresAt": expires})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		FullName *string `json:"fullName" validate:"omitempty,min=2,max=120"`
		RoleID   *string `json:"roleId"`
		Status   *string `json:"status" validate:"omitempty,oneof=active inactive"`
		// EmployeeID links the account to an employee record; "" removes the link.
		EmployeeID *string `json:"employeeId"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target, err := s.store.UserByID(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, notFoundOr(err, "User"))
		return
	}

	set, changes := bson.M{}, map[string]any{}
	if in.FullName != nil {
		set["fullName"] = strings.TrimSpace(*in.FullName)
		changes["fullName"] = set["fullName"]
	}
	roleChanges := false
	if in.RoleID != nil && *in.RoleID != target.RoleID.Hex() {
		roleID, err := bson.ObjectIDFromHex(*in.RoleID)
		if err == nil {
			_, err = s.store.RoleByID(r.Context(), orgID, roleID)
		}
		if err != nil {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"roleId": "is not a role in this organization"}))
			return
		}
		set["roleId"] = roleID
		changes["roleId"] = map[string]string{"from": target.RoleID.Hex(), "to": roleID.Hex()}
		roleChanges = true
	}
	deactivating := false
	if in.Status != nil && *in.Status != target.Status {
		if target.Status == domain.UserStatusInvited {
			httpx.Fail(w, r, httpx.Conflict("This user has not accepted their invitation yet."))
			return
		}
		set["status"] = *in.Status
		changes["status"] = map[string]string{"from": target.Status, "to": *in.Status}
		deactivating = *in.Status == domain.UserStatusInactive
	}
	unset := []string{}
	if in.EmployeeID != nil {
		if *in.EmployeeID == "" {
			unset = append(unset, "employeeId")
		} else {
			empID, err := bson.ObjectIDFromHex(*in.EmployeeID)
			if err == nil {
				_, err = s.store.EmployeeByID(r.Context(), orgID, empID)
			}
			if err != nil {
				httpx.Fail(w, r, httpx.Invalid(map[string]string{"employeeId": "is not an employee in this organization"}))
				return
			}
			set["employeeId"] = empID
		}
		changes["employeeId"] = *in.EmployeeID
	}
	if len(changes) == 0 {
		httpx.JSON(w, 200, target)
		return
	}

	if (roleChanges || deactivating) && target.ID == p.User.ID {
		httpx.Fail(w, r, httpx.Conflict("You cannot change your own role or deactivate your own account."))
		return
	}
	// Keep at least one active Organization Admin.
	if (roleChanges || deactivating) && target.Status == domain.UserStatusActive {
		role, err := s.store.RoleByID(r.Context(), orgID, target.RoleID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if role.Locked {
			n, err := s.store.CountActiveUsersWithRole(r.Context(), orgID, role.ID)
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if n <= 1 {
				httpx.Fail(w, r, httpx.Conflict("At least one active Organization Admin is required."))
				return
			}
		}
	}

	if err := s.store.UpdateUser(r.Context(), orgID, id, set, unset...); err != nil {
		httpx.Fail(w, r, notFoundOr(err, "User"))
		return
	}
	if deactivating {
		if err := s.store.DeleteUserSessions(r.Context(), id); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	s.audit(r, orgID, &p.User.ID, "user.updated", "user", id.Hex(), changes)
	updated, err := s.store.UserByID(r.Context(), orgID, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, updated)
}

func notFoundOr(err error, what string) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound(what)
	}
	return err
}
