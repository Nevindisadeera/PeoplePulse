package domain

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func twoStepSeparation(manager, hrRole bson.ObjectID) *Separation {
	s := &Separation{
		SubmissionDate: NewDate(2026, 11, 1), LastWorkingDay: NewDate(2026, 12, 1),
		ApprovalSteps: []ApprovalStep{
			{Order: 1, ApproverType: ApproverLineManager, UserID: &manager, Decision: DecisionPending},
			{Order: 2, ApproverType: ApproverRole, RoleID: &hrRole, Decision: DecisionPending},
		},
	}
	s.Start(bson.NewObjectID(), time.Now())
	return s
}

func TestApprovalChainHappyPath(t *testing.T) {
	manager, hr, hrRole, now := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), time.Now()
	s := twoStepSeparation(manager, hrRole)

	if s.Status != SeparationPending || s.PendingUserID == nil || *s.PendingUserID != manager || s.PendingRoleID != nil {
		t.Fatalf("after start: status %s, pending user %v role %v", s.Status, s.PendingUserID, s.PendingRoleID)
	}
	if step := s.PendingStep(); !step.AssignedTo(manager, bson.NewObjectID()) || step.AssignedTo(hr, hrRole) {
		t.Error("step 1 should belong to the line manager only")
	}

	final, err := s.Approve(manager, "ok", now)
	if err != nil || final {
		t.Fatalf("first approval: final=%v err=%v", final, err)
	}
	if s.Status != SeparationPending || s.PendingRoleID == nil || *s.PendingRoleID != hrRole || s.PendingUserID != nil {
		t.Errorf("after step 1: status %s, pending user %v role %v", s.Status, s.PendingUserID, s.PendingRoleID)
	}
	if !s.PendingStep().AssignedTo(hr, hrRole) {
		t.Error("step 2 should belong to anyone holding the HR role")
	}

	final, err = s.Approve(hr, "", now)
	if err != nil || !final || s.Status != SeparationApproved {
		t.Fatalf("second approval: final=%v err=%v status=%s", final, err, s.Status)
	}
	if s.PendingStep() != nil || s.PendingUserID != nil || s.PendingRoleID != nil {
		t.Error("an approved separation must have no pending approver")
	}
	if _, err := s.Approve(hr, "", now); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("approving an approved separation: %v", err)
	}
	if got := len(s.StatusHistory); got != 2 {
		t.Errorf("status history has %d entries, want 2 (pending, approved)", got)
	}
}

func TestRejectAndWithdraw(t *testing.T) {
	manager, hrRole, now := bson.NewObjectID(), bson.NewObjectID(), time.Now()

	s := twoStepSeparation(manager, hrRole)
	if err := s.Reject(manager, "need more notice", now); err != nil || s.Status != SeparationRejected {
		t.Fatalf("reject: %v, status %s", err, s.Status)
	}
	if s.ApprovalSteps[1].Decision != DecisionPending || s.PendingStep() != nil || s.PendingUserID != nil {
		t.Error("rejection must stop the chain without touching later steps")
	}
	for name, err := range map[string]error{
		"withdraw": s.Withdraw(manager, "", now),
		"reject":   s.Reject(manager, "x", now),
	} {
		if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("%s after rejection: %v", name, err)
		}
	}

	s = twoStepSeparation(manager, hrRole)
	if err := s.Withdraw(manager, "changed my mind", now); err != nil || s.Status != SeparationWithdrawn || s.PendingUserID != nil {
		t.Errorf("withdraw while pending: %v, status %s", err, s.Status)
	}
}

func TestNoChainIsApprovedImmediately(t *testing.T) {
	s := &Separation{ApprovalSteps: []ApprovalStep{}}
	s.Start(bson.NewObjectID(), time.Now())
	if s.Status != SeparationApproved {
		t.Errorf("status %s, want approved", s.Status)
	}
}

func TestComplete(t *testing.T) {
	by, dept, now := bson.NewObjectID(), bson.NewObjectID(), time.Now()
	emp := &Employee{DepartmentID: dept, JoinDate: NewDate(2024, 12, 1)}
	lastDay := NewDate(2026, 12, 1)

	pending := twoStepSeparation(by, by)
	if err := pending.Complete(by, emp, lastDay, now); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("completing a pending separation: %v", err)
	}

	s := &Separation{LastWorkingDay: lastDay, ApprovalSteps: []ApprovalStep{}}
	s.Start(by, now)
	if err := s.Complete(by, emp, lastDay.AddDays(-1), now); !errors.Is(err, ErrNotYetLastDay) {
		t.Errorf("completing the day before the last working day: %v", err)
	}
	if err := s.Complete(by, emp, lastDay, now); err != nil {
		t.Fatalf("completing on the last working day: %v", err)
	}
	// 2024-12-01 to 2026-12-01 is exactly two years, neither a leap day inside: 730 days.
	if s.Status != SeparationCompleted || *s.TenureDaysAtExit != 730 || *s.DepartmentIDAtExit != dept {
		t.Errorf("status %s tenure %d dept %v", s.Status, *s.TenureDaysAtExit, s.DepartmentIDAtExit)
	}
	if err := s.Withdraw(by, "", now); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("withdrawing a completed separation: %v", err)
	}
}

func TestDefaultReasonsCoverEveryCategory(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range DefaultSeparationReasons() {
		seen[r.Category] = true
	}
	for _, c := range []string{CategoryVoluntary, CategoryInvoluntary, CategoryOther} {
		if !seen[c] {
			t.Errorf("no default reason in category %s", c)
		}
	}
}
