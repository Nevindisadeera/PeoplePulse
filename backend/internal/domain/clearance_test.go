package domain

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestClearanceLifecycle(t *testing.T) {
	by, now := bson.NewObjectID(), time.Now()
	c := &ExitClearance{Items: []ClearanceItem{
		{ItemID: "a", Status: ItemPending}, {ItemID: "b", Status: ItemPending}, {ItemID: "c", Status: ItemPending},
	}}
	c.Recompute()
	if c.Status != ClearanceInProgress || c.OpenItems() != 3 || c.ReadyForCompletion() {
		t.Fatalf("new checklist: status %s, open %d", c.Status, c.OpenItems())
	}

	if err := c.UpdateItem("a", ItemCleared, "", by, now); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{ItemBlocked, ItemNotApplicable} {
		if err := c.UpdateItem("b", status, "", by, now); !errors.Is(err, ErrRemarksRequired) {
			t.Errorf("%s without remarks: %v", status, err)
		}
	}
	if err := c.UpdateItem("zzz", ItemCleared, "", by, now); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("unknown item: %v", err)
	}

	// A blocked item keeps the checklist open; "not applicable" closes an item.
	_ = c.UpdateItem("b", ItemBlocked, "laptop not returned", by, now)
	_ = c.UpdateItem("c", ItemNotApplicable, "no company card issued", by, now)
	if c.Status != ClearanceInProgress || c.OpenItems() != 1 {
		t.Errorf("with one blocked item: status %s, open %d", c.Status, c.OpenItems())
	}
	_ = c.UpdateItem("b", ItemCleared, "returned", by, now)
	if c.Status != ClearanceCompleted || !c.ReadyForCompletion() {
		t.Errorf("all items closed: status %s", c.Status)
	}

	// Reopening an item reopens the checklist; an added item does too.
	_ = c.UpdateItem("a", ItemPending, "", by, now)
	if c.Status != ClearanceInProgress {
		t.Errorf("after reopening an item: status %s", c.Status)
	}
	_ = c.UpdateItem("a", ItemCleared, "", by, now)
	_ = c.AddItem(ClearanceItem{ItemID: "d", Status: ItemPending})
	if c.Status != ClearanceInProgress || c.OpenItems() != 1 {
		t.Errorf("after adding an item: status %s, open %d", c.Status, c.OpenItems())
	}

	c.Override = &ClearanceOverride{By: by, At: now, Reason: "asset written off"}
	if !c.ReadyForCompletion() {
		t.Error("an override should allow completion with open items")
	}
}

func TestEmptyAndCancelledClearance(t *testing.T) {
	empty := &ExitClearance{}
	empty.Recompute()
	if empty.Status != ClearanceCompleted {
		t.Errorf("a checklist with no items should be complete, got %s", empty.Status)
	}

	c := &ExitClearance{Status: ClearanceCancelled, Items: []ClearanceItem{{ItemID: "a", Status: ItemPending}}}
	if err := c.UpdateItem("a", ItemCleared, "", bson.NewObjectID(), time.Now()); !errors.Is(err, ErrClearanceCancelled) {
		t.Errorf("updating a cancelled checklist: %v", err)
	}
	c.Recompute()
	if c.Status != ClearanceCancelled || c.ReadyForCompletion() {
		t.Errorf("cancelled checklist changed status to %s", c.Status)
	}
}

func TestClearanceItemAssignment(t *testing.T) {
	user, role, other := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	byUser := ClearanceItem{UserID: &user}
	byRole := ClearanceItem{RoleID: &role}
	if !byUser.AssignedTo(user, other) || byUser.AssignedTo(other, role) {
		t.Error("a user-assigned item belongs to that user only")
	}
	if !byRole.AssignedTo(other, role) || byRole.AssignedTo(user, other) {
		t.Error("a role-assigned item belongs to anyone holding the role")
	}
}
