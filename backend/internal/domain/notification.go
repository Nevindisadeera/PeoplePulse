package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Notification is an in-app message for one user. The API decides who is
// notified; email delivery of the same event is handed to n8n through the outbox.
type Notification struct {
	ID             bson.ObjectID `bson:"_id" json:"id"`
	OrganizationID bson.ObjectID `bson:"organizationId" json:"-"`
	UserID         bson.ObjectID `bson:"userId" json:"-"`
	Type           string        `bson:"type" json:"type"`
	Title          string        `bson:"title" json:"title"`
	Body           string        `bson:"body" json:"body"`
	// Link is an application path, for example /separations/{id}.
	Link       string     `bson:"link" json:"link"`
	EntityType string     `bson:"entityType" json:"entityType"`
	EntityID   string     `bson:"entityId" json:"entityId"`
	ReadAt     *time.Time `bson:"readAt,omitempty" json:"readAt,omitempty"`
	CreatedAt  time.Time  `bson:"createdAt" json:"createdAt"`
}

const (
	OutboxPending   = "pending"
	OutboxDelivered = "delivered"
	OutboxFailed    = "failed"  // gave up after the maximum number of attempts
	OutboxSkipped   = "skipped" // no webhook was configured when the event was created

	EventApprovalRequested     = "separation.approval_requested"
	EventSeparationSubmitted   = "separation.submitted"
	EventSeparationApproved    = "separation.approved"
	EventSeparationRejected    = "separation.rejected"
	EventSeparationWithdrawn   = "separation.withdrawn"
	EventLastWorkingDayChanged = "separation.last_working_day_changed"
	EventClearanceAssigned     = "clearance.assigned"
	EventUserInvited           = "user.invited"
	EventPasswordResetRequest  = "auth.password_reset_requested"
)

// OutboxEvent is a business event waiting to be delivered to the automation
// webhook. It is written in the same transaction as the change it describes,
// so an event exists if and only if the change was saved.
type OutboxEvent struct {
	ID             bson.ObjectID `bson:"_id"`
	EventID        string        `bson:"eventId"`
	OrganizationID bson.ObjectID `bson:"organizationId"`
	Type           string        `bson:"type"`
	Payload        bson.M        `bson:"payload"`
	// Sensitive events carry a single-use link. Their payload is erased once delivered.
	Sensitive     bool       `bson:"sensitive,omitempty"`
	Status        string     `bson:"status"`
	Attempts      int        `bson:"attempts"`
	NextAttemptAt time.Time  `bson:"nextAttemptAt"`
	LastError     string     `bson:"lastError,omitempty"`
	DeliveredAt   *time.Time `bson:"deliveredAt,omitempty"`
	CreatedAt     time.Time  `bson:"createdAt"`
}
