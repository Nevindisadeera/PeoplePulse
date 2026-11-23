// Package api wires HTTP routes to handlers.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/auth"
	"peoplepulse/internal/config"
	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	tokens  *auth.TokenMaker
	limiter *auth.Limiter
	// resetLimiter slows down repeated password reset requests.
	resetLimiter *auth.Limiter
}

func NewServer(cfg config.Config, st *store.Store) *Server {
	return &Server{
		cfg:          cfg,
		store:        st,
		tokens:       auth.NewTokenMaker(cfg.JWTSecret, cfg.AccessTTL),
		limiter:      auth.NewLimiter(5, 15*time.Minute),
		resetLimiter: auth.NewLimiter(5, 15*time.Minute),
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(requestID, s.logRequests, middleware.Recoverer, s.cors)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, 200, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.store.Ping(ctx); err != nil {
			httpx.JSON(w, 503, map[string]string{"status": "database unavailable"})
			return
		}
		httpx.JSON(w, 200, map[string]string{"status": "ready"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/refresh", s.handleRefresh)
		r.Post("/auth/logout", s.handleLogout)
		r.Post("/auth/accept-invite", s.handleAcceptInvite)
		r.Post("/auth/forgot-password", s.handleForgotPassword)
		r.Post("/auth/reset-password", s.handleResetPassword)

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)
			r.Get("/me", s.handleMe)
			r.Get("/notifications", s.handleListNotifications)
			r.Post("/notifications/read-all", s.handleReadAllNotifications)
			r.Post("/notifications/{id}/read", s.handleReadNotification)

			r.With(s.require(domain.PermUserRead)).Get("/users", s.handleListUsers)
			r.With(s.require(domain.PermUserManage)).Post("/users", s.handleInviteUser)
			r.With(s.require(domain.PermUserManage)).Patch("/users/{id}", s.handleUpdateUser)
			r.With(s.require(domain.PermUserManage)).Post("/users/{id}/reset-link", s.handleCreateResetLink)

			r.With(s.require(domain.PermRoleRead)).Get("/roles", s.handleListRoles)
			r.With(s.require(domain.PermRoleRead)).Get("/permissions", s.handleListPermissions)
			r.With(s.require(domain.PermRoleManage)).Post("/roles", s.handleCreateRole)
			r.With(s.require(domain.PermRoleManage)).Patch("/roles/{id}", s.handleUpdateRole)

			r.Get("/organization", s.handleGetOrganization)
			r.With(s.require(domain.PermOrgSettingsManage)).Patch("/organization", s.handleUpdateOrganization)
			r.With(s.require(domain.PermOrgSettingsManage)).Put("/settings/employment-types", s.handlePutEmploymentTypes)

			r.With(s.require(domain.PermDepartmentRead)).Get("/departments", s.handleListDepartments)
			r.With(s.require(domain.PermDepartmentRead)).Get("/departments/{id}", s.handleGetDepartment)
			r.With(s.require(domain.PermDepartmentManage)).Post("/departments", s.handleCreateDepartment)
			r.With(s.require(domain.PermDepartmentManage)).Patch("/departments/{id}", s.handleUpdateDepartment)
			r.With(s.require(domain.PermDepartmentManage)).Post("/departments/{id}/archive", s.handleArchiveDepartment)
			r.With(s.require(domain.PermDepartmentManage)).Post("/departments/{id}/restore", s.handleRestoreDepartment)

			readEmployees := s.require(domain.PermEmployeeRead, domain.PermEmployeeReadDept)
			r.With(readEmployees).Get("/employees", s.handleListEmployees)
			r.With(readEmployees).Get("/employees/{id}", s.handleGetEmployee)
			r.With(readEmployees).Get("/employees/{id}/timeline", s.handleEmployeeTimeline)
			r.With(s.require(domain.PermEmployeeManage)).Post("/employees", s.handleCreateEmployee)
			r.With(s.require(domain.PermEmployeeManage)).Post("/employees/import", s.handleImportEmployees)
			r.With(s.require(domain.PermEmployeeManage)).Patch("/employees/{id}", s.handleUpdateEmployee)
			r.With(s.require(domain.PermEmployeeManage)).Post("/employees/{id}/transfers", s.handleTransferEmployee)

			r.With(s.require(domain.PermOrgSettingsManage)).Put("/settings/separation-reasons", s.handlePutSeparationReasons)
			r.With(s.require(domain.PermOrgSettingsManage)).Put("/settings/approval-chain", s.handlePutApprovalChain)

			// Separations: visibility and step ownership are checked per record in the handlers.
			involved := s.require(domain.PermSeparationRead, domain.PermSeparationSubmit, domain.PermSeparationManage, domain.PermSeparationApprove)
			submitOrManage := s.require(domain.PermSeparationSubmit, domain.PermSeparationManage)
			r.With(involved).Get("/separations", s.handleListSeparations)
			r.With(involved).Get("/separations/{id}", s.handleGetSeparation)
			r.With(submitOrManage).Post("/separations", s.handleCreateSeparation)
			r.With(submitOrManage).Post("/separations/{id}/withdraw", s.handleWithdrawSeparation)
			r.With(s.require(domain.PermSeparationApprove)).Post("/separations/{id}/approve", s.handleApproveSeparation)
			r.With(s.require(domain.PermSeparationApprove)).Post("/separations/{id}/reject", s.handleRejectSeparation)
			r.With(s.require(domain.PermSeparationManage)).Patch("/separations/{id}/last-working-day", s.handleSetLastWorkingDay)
			r.With(s.require(domain.PermSeparationManage)).Post("/separations/{id}/complete", s.handleCompleteSeparation)

			// Clearance: any signed-in user can see their own tasks; record access is checked in the handlers.
			r.With(s.require(domain.PermOrgSettingsManage)).Put("/settings/clearance-template", s.handlePutClearanceTemplate)
			r.Get("/me/clearance-tasks", s.handleMyClearanceTasks)
			r.With(s.require(domain.PermClearanceRead, domain.PermClearanceManage)).Get("/clearances", s.handleListClearances)
			r.Get("/clearances/{id}", s.handleGetClearance)
			r.With(s.require(domain.PermClearanceManage, domain.PermClearanceUpdateOwn)).Patch("/clearances/{id}/items/{itemId}", s.handleUpdateClearanceItem)
			r.With(s.require(domain.PermClearanceManage)).Post("/clearances/{id}/items", s.handleAddClearanceItem)
			r.With(s.require(domain.PermClearanceOverride)).Post("/clearances/{id}/override", s.handleOverrideClearance)

			// Exit interviews: HR, plus the departing employee when self-service is on (checked in the handlers).
			interviewHR := s.require(domain.PermExitInterviewRead, domain.PermExitInterviewManage)
			r.With(interviewHR).Get("/settings/exit-interview-template", s.handleGetInterviewTemplate)
			r.With(s.require(domain.PermExitInterviewManage)).Put("/settings/exit-interview-template", s.handlePutInterviewTemplate)
			r.With(s.require(domain.PermExitInterviewManage)).Post("/separations/{id}/exit-interview", s.handleCreateExitInterview)
			r.Get("/separations/{id}/exit-interview", s.handleGetSeparationInterview)
			r.With(interviewHR).Get("/exit-interviews", s.handleListExitInterviews)
			r.Get("/exit-interviews/{id}", s.handleGetExitInterview)
			r.Patch("/exit-interviews/{id}", s.handleUpdateExitInterview)
			r.Post("/exit-interviews/{id}/submit", s.handleSubmitExitInterview)
			r.With(s.require(domain.PermExitInterviewRead)).Get("/analytics/exit-feedback", s.handleExitFeedback)

			// Analytics: department heads are limited to their own departments inside the handlers.
			analytics := s.require(domain.PermAnalyticsRead)
			r.With(analytics).Get("/analytics/turnover", s.handleTurnover)
			r.With(analytics).Get("/analytics/headcount", s.handleHeadcount)
			r.With(analytics).Get("/analytics/retention", s.handleRetention)
			r.With(analytics).Get("/analytics/tenure", s.handleTenure)
			r.With(s.require(domain.PermEmployeeManage)).Get("/analytics/data-quality", s.handleDataQuality)

			r.With(s.require(domain.PermReportExport)).Get("/reports/{type}", s.handleReport)
			r.With(s.require(domain.PermAuditRead)).Get("/audit-logs", s.handleListAuditLogs)
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) { httpx.Fail(w, r, httpx.NotFound("Route")) })
	return r
}

// ---- principal and middleware ----

type Principal struct {
	User  *domain.User
	Role  *domain.Role
	perms map[string]bool
}

func (p *Principal) Can(perm string) bool { return p.perms[perm] }

type ctxKey struct{}

func principal(r *http.Request) *Principal {
	p, _ := r.Context().Value(ctxKey{}).(*Principal)
	return p
}

// authenticate verifies the access token, then loads the user and role fresh
// so deactivation and permission changes take effect immediately.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || raw == "" {
			httpx.Fail(w, r, httpx.Unauthorized("Sign in to continue."))
			return
		}
		claims, err := s.tokens.Parse(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.Unauthorized("Your session has expired. Sign in again."))
			return
		}
		userID, err1 := bson.ObjectIDFromHex(claims.Subject)
		orgID, err2 := bson.ObjectIDFromHex(claims.Org)
		if err1 != nil || err2 != nil {
			httpx.Fail(w, r, httpx.Unauthorized("Sign in to continue."))
			return
		}
		user, err := s.store.UserByID(r.Context(), orgID, userID)
		if err != nil || user.Status != domain.UserStatusActive {
			httpx.Fail(w, r, httpx.Unauthorized("Sign in to continue."))
			return
		}
		role, err := s.store.RoleByID(r.Context(), orgID, user.RoleID)
		if err != nil {
			httpx.Fail(w, r, httpx.Forbidden())
			return
		}
		p := &Principal{User: user, Role: role, perms: map[string]bool{}}
		for _, k := range role.Permissions {
			p.perms[k] = true
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	})
}

// require lets the request through when the caller holds any of the permissions.
func (s *Server) require(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := principal(r); p != nil {
				for _, perm := range perms {
					if p.Can(perm) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			httpx.Fail(w, r, httpx.Forbidden())
		})
	}
}

// requestID tags each request with a random ID. chi's built-in version embeds
// the server's hostname, which should not be sent to clients.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id := "req_" + hex.EncodeToString(b)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), middleware.RequestIDKey, id)))
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		slog.Info("request", "requestId", middleware.GetReqID(r.Context()), "method", r.Method,
			"path", r.URL.Path, "status", ww.Status(), "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin == s.cfg.CORSOrigin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- shared helpers ----

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// audit records a business action. A failed audit write is logged, not returned.
func (s *Server) audit(r *http.Request, orgID bson.ObjectID, actor *bson.ObjectID, action, entityType, entityID string, changes map[string]any) {
	entry := &domain.AuditLog{
		ID: bson.NewObjectID(), OrganizationID: orgID, ActorID: actor, ActorType: "user",
		Action: action, EntityType: entityType, EntityID: entityID, Changes: changes,
		IP: clientIP(r), At: time.Now().UTC(),
	}
	if err := s.store.InsertAudit(r.Context(), entry); err != nil {
		slog.Error("audit write failed", "action", action, "err", err)
	}
}

func pathID(r *http.Request) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		return id, httpx.NotFound("Record")
	}
	return id, nil
}
