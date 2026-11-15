package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

// event is one thing that happened, and who should hear about it.
type event struct {
	Type       string
	Title      string
	Body       string
	Link       string // application path
	EntityType string
	EntityID   string
	Recipients []bson.ObjectID // user IDs; duplicates and the actor are removed
	Data       bson.M          // extra fields for the automation payload
}

// emit stores an in-app notification for each recipient and one outbox event
// for email delivery. It is called inside the same Atomic block as the
// business change, so notifications exist if and only if the change was saved.
// The person who performed the action is not notified about it.
func (s *Server) emit(ctx context.Context, orgID, actor bson.ObjectID, ev event) error {
	ids := []bson.ObjectID{}
	for _, id := range ev.Recipients {
		if id != actor && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	users, err := s.store.ActiveUsersByIDs(ctx, orgID, ids)
	if err != nil || len(users) == 0 {
		return err
	}
	now := time.Now().UTC()
	notifications := make([]domain.Notification, len(users))
	recipients := make(bson.A, len(users))
	for i, u := range users {
		notifications[i] = domain.Notification{
			ID: bson.NewObjectID(), OrganizationID: orgID, UserID: u.ID, Type: ev.Type, Title: ev.Title, Body: ev.Body,
			Link: ev.Link, EntityType: ev.EntityType, EntityID: ev.EntityID, CreatedAt: now,
		}
		recipients[i] = bson.M{"userId": u.ID.Hex(), "email": u.Email, "name": u.FullName}
	}
	if err := s.store.InsertNotifications(ctx, notifications); err != nil {
		return err
	}
	payload := bson.M{"title": ev.Title, "body": ev.Body, "link": s.cfg.AppBaseURL + ev.Link,
		"entityType": ev.EntityType, "entityId": ev.EntityID, "recipients": recipients}
	for k, v := range ev.Data {
		payload[k] = v
	}
	status := domain.OutboxPending
	if s.cfg.WebhookURL == "" {
		status = domain.OutboxSkipped // nothing to deliver to; do not build a backlog
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	return s.store.InsertOutboxEvent(ctx, &domain.OutboxEvent{
		ID: bson.NewObjectID(), EventID: "evt_" + hex.EncodeToString(id), OrganizationID: orgID, Type: ev.Type,
		Payload: payload, Status: status, NextAttemptAt: now, CreatedAt: now,
	})
}

// ---- recipient lookups ----

func (s *Server) usersWithPermission(ctx context.Context, orgID bson.ObjectID, perm string) ([]bson.ObjectID, error) {
	roles, err := s.store.RoleIDsWithPermission(ctx, orgID, perm)
	if err != nil {
		return nil, err
	}
	return s.store.ActiveUserIDsWithRoles(ctx, orgID, roles)
}

// assignedUsers resolves an assignment (a user, or everyone holding a role) to user IDs.
func (s *Server) assignedUsers(ctx context.Context, orgID bson.ObjectID, userID, roleID *bson.ObjectID) ([]bson.ObjectID, error) {
	switch {
	case userID != nil:
		return []bson.ObjectID{*userID}, nil
	case roleID != nil:
		return s.store.ActiveUserIDsWithRoles(ctx, orgID, []bson.ObjectID{*roleID})
	}
	return nil, nil
}

func (s *Server) pendingApprovers(ctx context.Context, sep *domain.Separation) ([]bson.ObjectID, error) {
	return s.assignedUsers(ctx, sep.OrganizationID, sep.PendingUserID, sep.PendingRoleID)
}

// employeeUser returns the user account of an employee, if they have one.
func (s *Server) employeeUser(ctx context.Context, orgID, employeeID bson.ObjectID) ([]bson.ObjectID, error) {
	u, err := s.store.ActiveUserByEmployeeID(ctx, orgID, employeeID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []bson.ObjectID{u.ID}, nil
}

// ---- separation events ----

type sepContext struct {
	sep  *domain.Separation
	name string // "Full Name (E001)"
	link string
	data bson.M
}

func (s *Server) sepContext(ctx context.Context, sep *domain.Separation) (sepContext, error) {
	emp, err := s.store.EmployeeByID(ctx, sep.OrganizationID, sep.EmployeeID)
	if err != nil {
		return sepContext{}, err
	}
	// The payload carries what an email needs and nothing sensitive: no reason or reason details.
	return sepContext{sep: sep, name: emp.FullName + " (" + emp.EmployeeNumber + ")", link: "/separations/" + sep.ID.Hex(),
		data: bson.M{"separationId": sep.ID.Hex(), "employeeName": emp.FullName, "employeeNumber": emp.EmployeeNumber,
			"separationType": sep.Type, "lastWorkingDay": sep.LastWorkingDay.String(), "status": sep.Status}}, nil
}

func (c sepContext) event(kind, title, body string, recipients []bson.ObjectID) event {
	return event{Type: kind, Title: title, Body: body, Link: c.link, EntityType: "separation", EntityID: c.sep.ID.Hex(), Recipients: recipients, Data: c.data}
}

// notifyApprovalRequested tells the approver(s) of the pending step that it is their turn.
func (s *Server) notifyApprovalRequested(ctx context.Context, actor bson.ObjectID, c sepContext) ([]bson.ObjectID, error) {
	approvers, err := s.pendingApprovers(ctx, c.sep)
	if err != nil {
		return nil, err
	}
	return approvers, s.emit(ctx, c.sep.OrganizationID, actor, c.event(domain.EventApprovalRequested,
		"Approval needed: "+c.name, "A resignation from "+c.name+" is waiting for your decision. Requested last working day: "+c.sep.LastWorkingDay.String()+".", approvers))
}

// notifyEmployeeAndSubmitter: the employee is only told about their own
// resignation. Other separation types are communicated by HR in person, not by the system.
func (s *Server) employeeAndSubmitter(ctx context.Context, sep *domain.Separation) ([]bson.ObjectID, error) {
	to := []bson.ObjectID{}
	if sep.Type == domain.SeparationTypeResignation {
		own, err := s.employeeUser(ctx, sep.OrganizationID, sep.EmployeeID)
		if err != nil {
			return nil, err
		}
		to = append(own, sep.SubmittedBy)
	}
	return to, nil
}

// notifySeparationStarted runs when a separation is created.
func (s *Server) notifySeparationStarted(ctx context.Context, actor bson.ObjectID, sep *domain.Separation) error {
	c, err := s.sepContext(ctx, sep)
	if err != nil {
		return err
	}
	if sep.Status == domain.SeparationApproved {
		return s.notifyApproved(ctx, actor, c)
	}
	approvers, err := s.notifyApprovalRequested(ctx, actor, c)
	if err != nil {
		return err
	}
	// HR is told about every new resignation; approvers already have their own message.
	hr, err := s.usersWithPermission(ctx, sep.OrganizationID, domain.PermSeparationManage)
	if err != nil {
		return err
	}
	hr = slices.DeleteFunc(hr, func(id bson.ObjectID) bool { return slices.Contains(approvers, id) })
	return s.emit(ctx, sep.OrganizationID, actor, c.event(domain.EventSeparationSubmitted,
		"Resignation submitted: "+c.name, c.name+" has submitted a resignation. Requested last working day: "+sep.LastWorkingDay.String()+".", hr))
}

func (s *Server) notifyApproved(ctx context.Context, actor bson.ObjectID, c sepContext) error {
	to, err := s.employeeAndSubmitter(ctx, c.sep)
	if err != nil {
		return err
	}
	return s.emit(ctx, c.sep.OrganizationID, actor, c.event(domain.EventSeparationApproved,
		"Resignation approved: "+c.name, "The resignation of "+c.name+" has been approved. Last working day: "+c.sep.LastWorkingDay.String()+".", to))
}

// notifyDecision runs after an approval step is approved or rejected.
func (s *Server) notifyDecision(ctx context.Context, actor bson.ObjectID, sep *domain.Separation) error {
	c, err := s.sepContext(ctx, sep)
	if err != nil {
		return err
	}
	switch sep.Status {
	case domain.SeparationPending:
		_, err = s.notifyApprovalRequested(ctx, actor, c)
		return err
	case domain.SeparationApproved:
		return s.notifyApproved(ctx, actor, c)
	}
	to, err := s.employeeAndSubmitter(ctx, sep)
	if err != nil {
		return err
	}
	return s.emit(ctx, sep.OrganizationID, actor, c.event(domain.EventSeparationRejected,
		"Resignation not approved: "+c.name, "The resignation of "+c.name+" was not approved. Open it to see the approver's comment.", to))
}

// notifyWithdrawn tells HR and whoever had been asked to approve.
func (s *Server) notifyWithdrawn(ctx context.Context, actor bson.ObjectID, sep *domain.Separation, wasAwaiting []bson.ObjectID) error {
	c, err := s.sepContext(ctx, sep)
	if err != nil {
		return err
	}
	hr, err := s.usersWithPermission(ctx, sep.OrganizationID, domain.PermSeparationManage)
	if err != nil {
		return err
	}
	own, err := s.employeeAndSubmitter(ctx, sep)
	if err != nil {
		return err
	}
	return s.emit(ctx, sep.OrganizationID, actor, c.event(domain.EventSeparationWithdrawn,
		"Separation withdrawn: "+c.name, "The separation of "+c.name+" has been withdrawn. No further action is needed.", slices.Concat(hr, wasAwaiting, own)))
}

func (s *Server) notifyLastWorkingDayChanged(ctx context.Context, actor bson.ObjectID, sep *domain.Separation, from domain.Date) error {
	c, err := s.sepContext(ctx, sep)
	if err != nil {
		return err
	}
	to, err := s.employeeAndSubmitter(ctx, sep)
	if err != nil {
		return err
	}
	c.data["previousLastWorkingDay"] = from.String()
	return s.emit(ctx, sep.OrganizationID, actor, c.event(domain.EventLastWorkingDayChanged,
		"Last working day changed: "+c.name, "The last working day of "+c.name+" changed from "+from.String()+" to "+sep.LastWorkingDay.String()+".", to))
}

// notifyClearanceAssigned tells the assignees of the given items that they have work to do.
func (s *Server) notifyClearanceAssigned(ctx context.Context, actor bson.ObjectID, c *domain.ExitClearance, emp *domain.Employee, items []domain.ClearanceItem) error {
	to := []bson.ObjectID{}
	due := domain.Date{}
	for _, item := range items {
		ids, err := s.assignedUsers(ctx, c.OrganizationID, item.UserID, item.RoleID)
		if err != nil {
			return err
		}
		to = append(to, ids...)
		if due.IsZero() || item.DueDate.Before(due) {
			due = item.DueDate
		}
	}
	if len(to) == 0 {
		return nil
	}
	name := emp.FullName + " (" + emp.EmployeeNumber + ")"
	return s.emit(ctx, c.OrganizationID, actor, event{
		Type: domain.EventClearanceAssigned, Title: "Clearance tasks: " + name,
		Body: "You have exit clearance items to complete for " + name + ", due by " + due.String() + ".",
		Link: "/clearances/" + c.ID.Hex(), EntityType: "clearance", EntityID: c.ID.Hex(), Recipients: to,
		Data: bson.M{"clearanceId": c.ID.Hex(), "separationId": c.SeparationID.Hex(), "employeeName": emp.FullName,
			"employeeNumber": emp.EmployeeNumber, "dueDate": due.String()},
	})
}

// ---- notification endpoints (each user sees only their own) ----

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	page, size := httpx.Page(r)
	list, total, err := s.store.ListNotifications(r.Context(), p.User.OrganizationID, p.User.ID, r.URL.Query().Get("unread") == "true", page, size)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	unread, err := s.store.CountUnreadNotifications(r.Context(), p.User.OrganizationID, p.User.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": list, "page": page, "pageSize": size, "total": total, "unreadCount": unread})
}

func (s *Server) handleReadNotification(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	id, err := pathID(r)
	if err == nil {
		err = s.store.MarkNotificationRead(r.Context(), p.User.OrganizationID, p.User.ID, id)
	}
	if err != nil {
		httpx.Fail(w, r, storeErr(err, "Notification"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReadAllNotifications(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	n, err := s.store.MarkAllNotificationsRead(r.Context(), p.User.OrganizationID, p.User.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"marked": n})
}
