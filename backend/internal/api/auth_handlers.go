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

const refreshCookie = "pp_refresh"

// Verified against when the email is unknown, so timing does not reveal which emails exist.
var dummyHash, _ = auth.HashPassword("peoplepulse-dummy-password")

type authResponse struct {
	AccessToken string       `json:"accessToken"`
	ExpiresAt   time.Time    `json:"expiresAt"`
	User        *domain.User `json:"user"`
	Role        *domain.Role `json:"role"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	key := clientIP(r) + "|" + email
	if !s.limiter.Allow(key) {
		httpx.Fail(w, r, httpx.TooMany())
		return
	}

	user, err := s.store.UserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		httpx.Fail(w, r, err)
		return
	}
	hash := dummyHash
	if err == nil && user.PasswordHash != "" {
		hash = user.PasswordHash
	}
	if !auth.VerifyPassword(in.Password, hash) || err != nil || user.Status != domain.UserStatusActive {
		httpx.Fail(w, r, httpx.Unauthorized("Email or password is incorrect."))
		return
	}
	s.limiter.Reset(key)

	now := time.Now().UTC()
	if err := s.store.UpdateUser(r.Context(), user.OrganizationID, user.ID, bson.M{"lastLoginAt": now}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	user.LastLoginAt = &now
	if err := s.startSession(w, r, user, bson.NewObjectID()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, user.OrganizationID, &user.ID, "auth.login", "user", user.ID.Hex(), nil)
	s.writeAuth(w, r, user)
}

// handleRefresh rotates the refresh token. Presenting an already-used token
// ends every session descended from the same login.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	fail := func() {
		s.clearCookie(w)
		httpx.Fail(w, r, httpx.Unauthorized("Your session has expired. Sign in again."))
	}
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		fail()
		return
	}
	sess, err := s.store.SessionByHash(r.Context(), auth.HashToken(c.Value))
	if err != nil || time.Now().After(sess.ExpiresAt) {
		fail()
		return
	}
	fresh, err := s.store.MarkSessionRotated(r.Context(), sess.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !fresh {
		_ = s.store.DeleteSessionFamily(r.Context(), sess.FamilyID)
		fail()
		return
	}
	user, err := s.store.UserByID(r.Context(), sess.OrganizationID, sess.UserID)
	if err != nil || user.Status != domain.UserStatusActive {
		_ = s.store.DeleteSessionFamily(r.Context(), sess.FamilyID)
		fail()
		return
	}
	if err := s.startSession(w, r, user, sess.FamilyID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.writeAuth(w, r, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if sess, err := s.store.SessionByHash(r.Context(), auth.HashToken(c.Value)); err == nil {
			_ = s.store.DeleteSessionFamily(r.Context(), sess.FamilyID)
			s.audit(r, sess.OrganizationID, &sess.UserID, "auth.logout", "user", sess.UserID.Hex(), nil)
		}
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	user, err := s.store.UserByInviteHash(r.Context(), auth.HashToken(in.Token))
	if err != nil || user.Status != domain.UserStatusInvited ||
		user.InviteExpiresAt == nil || time.Now().After(*user.InviteExpiresAt) {
		httpx.Fail(w, r, httpx.BadRequest("This invitation link is invalid or has expired. Ask your administrator for a new one."))
		return
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"password": err.Error()}))
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = s.store.UpdateUser(r.Context(), user.OrganizationID, user.ID,
		bson.M{"passwordHash": hash, "status": domain.UserStatusActive}, "inviteTokenHash", "inviteExpiresAt")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, user.OrganizationID, &user.ID, "user.invite_accepted", "user", user.ID.Hex(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	httpx.JSON(w, 200, map[string]any{"user": p.User, "role": p.Role})
}

// startSession stores a new refresh token in the given family and sets the cookie.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *domain.User, family bson.ObjectID) error {
	token, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	sess := &domain.Session{
		ID: bson.NewObjectID(), OrganizationID: u.OrganizationID, UserID: u.ID, FamilyID: family,
		TokenHash: hash, UserAgent: r.UserAgent(), ExpiresAt: now.Add(s.cfg.RefreshTTL), CreatedAt: now,
	}
	if err := s.store.InsertSession(r.Context(), sess); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: token, Path: "/api/v1/auth", Expires: sess.ExpiresAt,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/api/v1/auth", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) writeAuth(w http.ResponseWriter, r *http.Request, u *domain.User) {
	role, err := s.store.RoleByID(r.Context(), u.OrganizationID, u.RoleID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tok, exp, err := s.tokens.Issue(u.ID.Hex(), u.OrganizationID.Hex(), u.RoleID.Hex())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, authResponse{AccessToken: tok, ExpiresAt: exp, User: u, Role: role})
}
