package domain

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	ClearanceInProgress = "in_progress"
	ClearanceCompleted  = "completed"
	ClearanceCancelled  = "cancelled"

	ItemPending       = "pending"
	ItemCleared       = "cleared"
	ItemNotApplicable = "not_applicable"
	ItemBlocked       = "blocked"
)

var ItemStatuses = []string{ItemPending, ItemCleared, ItemNotApplicable, ItemBlocked}

// ClearanceTemplateItem is one line of the organization's clearance checklist
// template. The assignee is a role, a named user or the employee's line manager.
type ClearanceTemplateItem struct {
	Category     string         `bson:"category" json:"category"`
	Title        string         `bson:"title" json:"title"`
	AssigneeType string         `bson:"assigneeType" json:"assigneeType"`
	RoleID       *bson.ObjectID `bson:"roleId,omitempty" json:"roleId,omitempty"`
	UserID       *bson.ObjectID `bson:"userId,omitempty" json:"userId,omitempty"`
	Active       bool           `bson:"active" json:"active"`
}

// ClearanceItem is a template line copied onto one employee's checklist, with
// the assignee resolved.
type ClearanceItem struct {
	ItemID       string         `bson:"itemId" json:"itemId"`
	Category     string         `bson:"category" json:"category"`
	Title        string         `bson:"title" json:"title"`
	AssigneeType string         `bson:"assigneeType" json:"assigneeType"`
	RoleID       *bson.ObjectID `bson:"roleId,omitempty" json:"roleId,omitempty"`
	UserID       *bson.ObjectID `bson:"userId,omitempty" json:"userId,omitempty"`
	Fallback     bool           `bson:"fallback,omitempty" json:"fallback,omitempty"`
	Status       string         `bson:"status" json:"status"`
	Remarks      string         `bson:"remarks,omitempty" json:"remarks,omitempty"`
	DueDate      Date           `bson:"dueDate" json:"dueDate"`
	UpdatedBy    *bson.ObjectID `bson:"updatedBy,omitempty" json:"updatedBy,omitempty"`
	UpdatedAt    *time.Time     `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

func (i ClearanceItem) AssignedTo(userID, roleID bson.ObjectID) bool {
	if i.UserID != nil {
		return *i.UserID == userID
	}
	return i.RoleID != nil && *i.RoleID == roleID
}

// Open reports whether the item still needs action.
func (i ClearanceItem) Open() bool { return i.Status == ItemPending || i.Status == ItemBlocked }

type ClearanceOverride struct {
	By     bson.ObjectID `bson:"by" json:"by"`
	At     time.Time     `bson:"at" json:"at"`
	Reason string        `bson:"reason" json:"reason"`
}

type ExitClearance struct {
	ID             bson.ObjectID   `bson:"_id" json:"id"`
	OrganizationID bson.ObjectID   `bson:"organizationId" json:"-"`
	SeparationID   bson.ObjectID   `bson:"separationId" json:"separationId"`
	EmployeeID     bson.ObjectID   `bson:"employeeId" json:"employeeId"`
	Status         string          `bson:"status" json:"status"`
	Items          []ClearanceItem `bson:"items" json:"items"`
	// Override lets HR complete the separation while items are still open.
	Override  *ClearanceOverride `bson:"override,omitempty" json:"override,omitempty"`
	Version   int                `bson:"version" json:"version"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

var (
	ErrClearanceCancelled = errors.New("clearance was cancelled")
	ErrItemNotFound       = errors.New("clearance item not found")
	ErrRemarksRequired    = errors.New("remarks are required for this status")
)

func NewItemID() string { return bson.NewObjectID().Hex() }

func (c *ExitClearance) OpenItems() int {
	n := 0
	for _, i := range c.Items {
		if i.Open() {
			n++
		}
	}
	return n
}

// Recompute derives the checklist status from its items: completed when every
// item is cleared or not applicable.
func (c *ExitClearance) Recompute() {
	if c.Status == ClearanceCancelled {
		return
	}
	if c.OpenItems() == 0 {
		c.Status = ClearanceCompleted
	} else {
		c.Status = ClearanceInProgress
	}
}

// ReadyForCompletion reports whether the separation may be completed.
func (c *ExitClearance) ReadyForCompletion() bool {
	return c.Status == ClearanceCompleted || c.Override != nil
}

func (c *ExitClearance) Item(itemID string) *ClearanceItem {
	for i := range c.Items {
		if c.Items[i].ItemID == itemID {
			return &c.Items[i]
		}
	}
	return nil
}

// UpdateItem sets an item's status. "Blocked" and "not applicable" must be explained.
func (c *ExitClearance) UpdateItem(itemID, status, remarks string, by bson.ObjectID, now time.Time) error {
	if c.Status == ClearanceCancelled {
		return ErrClearanceCancelled
	}
	item := c.Item(itemID)
	if item == nil {
		return ErrItemNotFound
	}
	if (status == ItemBlocked || status == ItemNotApplicable) && remarks == "" {
		return ErrRemarksRequired
	}
	item.Status, item.Remarks, item.UpdatedBy, item.UpdatedAt = status, remarks, &by, &now
	c.Recompute()
	return nil
}

func (c *ExitClearance) AddItem(item ClearanceItem) error {
	if c.Status == ClearanceCancelled {
		return ErrClearanceCancelled
	}
	c.Items = append(c.Items, item)
	c.Recompute()
	return nil
}
