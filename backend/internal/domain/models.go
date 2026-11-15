// Package domain holds the core data types shared across the backend.
package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	UserStatusInvited  = "invited"
	UserStatusActive   = "active"
	UserStatusInactive = "inactive"
)

type Organization struct {
	ID        bson.ObjectID `bson:"_id" json:"id"`
	Name      string        `bson:"name" json:"name"`
	TimeZone  string        `bson:"timeZone" json:"timeZone"`
	CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt" json:"updatedAt"`
}

type Role struct {
	ID              bson.ObjectID `bson:"_id" json:"id"`
	OrganizationID  bson.ObjectID `bson:"organizationId" json:"-"`
	Name            string        `bson:"name" json:"name"`
	Description     string        `bson:"description" json:"description"`
	Permissions     []string      `bson:"permissions" json:"permissions"`
	IsSystemDefault bool          `bson:"isSystemDefault" json:"isSystemDefault"`
	// Locked roles (Organization Admin) cannot have their permissions edited.
	Locked    bool      `bson:"locked" json:"locked"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

type User struct {
	ID              bson.ObjectID  `bson:"_id" json:"id"`
	OrganizationID  bson.ObjectID  `bson:"organizationId" json:"-"`
	Email           string         `bson:"email" json:"email"`
	PasswordHash    string         `bson:"passwordHash,omitempty" json:"-"`
	FullName        string         `bson:"fullName" json:"fullName"`
	RoleID          bson.ObjectID  `bson:"roleId" json:"roleId"`
	EmployeeID      *bson.ObjectID `bson:"employeeId,omitempty" json:"employeeId,omitempty"`
	Status          string         `bson:"status" json:"status"`
	InviteTokenHash string         `bson:"inviteTokenHash,omitempty" json:"-"`
	InviteExpiresAt *time.Time     `bson:"inviteExpiresAt,omitempty" json:"-"`
	LastLoginAt     *time.Time     `bson:"lastLoginAt,omitempty" json:"lastLoginAt,omitempty"`
	CreatedAt       time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time      `bson:"updatedAt" json:"updatedAt"`
}

// Session is one refresh token. Tokens in the same family descend from one login.
type Session struct {
	ID             bson.ObjectID `bson:"_id"`
	OrganizationID bson.ObjectID `bson:"organizationId"`
	UserID         bson.ObjectID `bson:"userId"`
	FamilyID       bson.ObjectID `bson:"familyId"`
	TokenHash      string        `bson:"tokenHash"`
	UserAgent      string        `bson:"userAgent"`
	RotatedAt      *time.Time    `bson:"rotatedAt,omitempty"`
	ExpiresAt      time.Time     `bson:"expiresAt"`
	CreatedAt      time.Time     `bson:"createdAt"`
}

type AuditLog struct {
	ID             bson.ObjectID  `bson:"_id" json:"id"`
	OrganizationID bson.ObjectID  `bson:"organizationId" json:"-"`
	ActorID        *bson.ObjectID `bson:"actorId,omitempty" json:"actorId,omitempty"`
	ActorType      string         `bson:"actorType" json:"actorType"` // user | integration | system
	Action         string         `bson:"action" json:"action"`
	EntityType     string         `bson:"entityType" json:"entityType"`
	EntityID       string         `bson:"entityId" json:"entityId"`
	Changes        map[string]any `bson:"changes,omitempty" json:"changes,omitempty"`
	IP             string         `bson:"ip" json:"ip"`
	At             time.Time      `bson:"at" json:"at"`
}
