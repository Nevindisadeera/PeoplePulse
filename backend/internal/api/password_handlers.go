package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/auth"
	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

// sendLink queues an email that carries a single-use link (an invitation or a
// password reset). Nothing is stored when no webhook is configured: a live
// link must not sit in the database with nowhere to go. A failure here is
// logged and never fails the request that triggered it.
func (s *Server) sendLink(r *http.Request, u *domain.User, kind, title, body, path string) {
	if s.cfg.WebhookURL == "" {
		return
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		slog.Error("could not queue link email", "event", kind, "err", err)
		return
	}
	now := time.Now().UTC()
	err := s.store.InsertOutboxEvent(r.Context(), &domain.OutboxEvent{
		ID: bson.NewObjectID(), EventID: "evt_" + hex.EncodeToString(id), OrganizationID: u.OrganizationID, Type: kind,
		Sensitive: true, Status: domain.OutboxPending, NextAttemptAt: now, CreatedAt: now,
		Payload: bson.M{"title": title, "body": body, "link": s.cfg.AppBaseURL + path,
			"recipients": bson.A{bson.M{"userId": u.ID.Hex(), "email": u.Email, "name": u.FullName}}},
	})
	if err != nil {
		slog.Error("could not queue link email", "event", kind, "err", err)
	}
}

// newResetToken stores a fresh single-use reset token on the user and returns it.
func (s *Server) newResetToken(r *http.Request, u *domain.User) (string, time.Time, error) {
	token, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(s.cfg.ResetTTL)
	err = s.store.UpdateUser(r.Context(), u.OrganizationID, u.ID, bson.M{"resetTokenHash": hash, "resetExpiresAt": expires})
	return token, expires, err
}

// handleForgotPassword always answers the same way, whether or not the email
// belongs to an account, so the endpoint cannot be used to discover accounts.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email" validate:"required,email"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !s.resetLimiter.Allow(clientIP(r)+"|"+email) || !s.resetLimiter.Allow("ip|"+clientIP(r)) {
		httpx.Fail(w, r, httpx.TooMany())
		return
	}
	user, err := s.store.UserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		httpx.Fail(w, r, err)
		return
	}
	if err == nil && user.Status == domain.UserStatusActive {
		token, _, err := s.newResetToken(r, user)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		s.audit(r, user.OrganizationID, &user.ID, "auth.password_reset_requested", "user", user.ID.Hex(), nil)
		s.sendLink(r, user, domain.EventPasswordResetRequest, "Reset your PeoplePulse password",
			"Someone asked to reset the password for this account. If it was you, use the link within one hour. If it was not, ignore this message; your password has not changed.",
			"/reset-password?token="+token)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleResetPassword sets a new password from a reset token and signs the
// user out everywhere.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	user, err := s.store.UserByResetHash(r.Context(), auth.HashToken(in.Token))
	if err != nil || user.Status != domain.UserStatusActive || user.ResetExpiresAt == nil || time.Now().After(*user.ResetExpiresAt) {
		httpx.Fail(w, r, httpx.BadRequest("This reset link is invalid or has expired. Request a new one."))
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
	if err := s.store.UpdateUser(r.Context(), user.OrganizationID, user.ID, bson.M{"passwordHash": hash}, "resetTokenHash", "resetExpiresAt"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Whoever knew the old password, or held a session, is signed out.
	if err := s.store.DeleteUserSessions(r.Context(), user.ID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, user.OrganizationID, &user.ID, "auth.password_reset", "user", user.ID.Hex(), nil)
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateResetLink lets an administrator produce a reset link to pass on
// in person. It is the fallback when email delivery is not set up.
func (s *Server) handleCreateResetLink(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	user, err := s.store.UserByID(r.Context(), p.User.OrganizationID, id)
	if err != nil {
		httpx.Fail(w, r, notFoundOr(err, "User"))
		return
	}
	if user.Status != domain.UserStatusActive {
		httpx.Fail(w, r, httpx.Conflict("Only an active user can reset their password. An invited user needs a new invitation."))
		return
	}
	token, expires, err := s.newResetToken(r, user)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, user.OrganizationID, &p.User.ID, "user.reset_link_created", "user", user.ID.Hex(), nil)
	httpx.JSON(w, 201, map[string]any{"resetToken": token, "expiresAt": expires})
}
