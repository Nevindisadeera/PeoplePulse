package domain

import (
	"errors"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	SeparationPending   = "pending_approval"
	SeparationApproved  = "approved"
	SeparationRejected  = "rejected"
	SeparationWithdrawn = "withdrawn"
	SeparationCompleted = "completed"

	SeparationTypeResignation = "resignation"

	CategoryVoluntary   = "voluntary"
	CategoryInvoluntary = "involuntary"
	CategoryOther       = "other"

	ApproverLineManager = "line_manager"
	ApproverRole        = "role"
	ApproverUser        = "user"

	DecisionPending  = "pending"
	DecisionApproved = "approved"
	DecisionRejected = "rejected"

	MaxApprovalSteps = 3
)

// SeparationTypes lists every way an employment period can end. Only a
// resignation goes through the approval chain; HR records the others.
var SeparationTypes = []string{SeparationTypeResignation, "termination", "redundancy", "retirement", "end_of_contract", "other"}

// OpenSeparationStatuses are the statuses in which a separation is still in progress.
var OpenSeparationStatuses = []string{SeparationPending, SeparationApproved}

// SeparationReason is an organization-defined reason. Its category decides
// whether a separation counts as voluntary, involuntary or other turnover.
type SeparationReason struct {
	Code     string `bson:"code" json:"code"`
	Name     string `bson:"name" json:"name"`
	Category string `bson:"category" json:"category"`
	Active   bool   `bson:"active" json:"active"`
}

func DefaultSeparationReasons() []SeparationReason {
	return []SeparationReason{
		{"better_opportunity", "Better opportunity elsewhere", CategoryVoluntary, true},
		{"personal", "Personal or family reasons", CategoryVoluntary, true},
		{"relocation", "Relocation", CategoryVoluntary, true},
		{"further_studies", "Further studies", CategoryVoluntary, true},
		{"work_environment", "Work environment or management", CategoryVoluntary, true},
		{"performance", "Termination: performance", CategoryInvoluntary, true},
		{"misconduct", "Termination: misconduct", CategoryInvoluntary, true},
		{"redundancy", "Redundancy", CategoryInvoluntary, true},
		{"retirement", "Retirement", CategoryOther, true},
		{"contract_end", "End of fixed-term contract", CategoryOther, true},
	}
}

// ChainStep is one configured approval step. RoleID is set for a role step,
// UserID for a user step, and neither for the employee's line manager.
type ChainStep struct {
	ApproverType string         `bson:"approverType" json:"approverType"`
	RoleID       *bson.ObjectID `bson:"roleId,omitempty" json:"roleId,omitempty"`
	UserID       *bson.ObjectID `bson:"userId,omitempty" json:"userId,omitempty"`
}

// ApprovalStep is a chain step copied onto a separation at submission, with
// the approver resolved. Later changes to the organization's chain do not
// affect requests already in progress.
type ApprovalStep struct {
	Order        int            `bson:"order" json:"order"`
	ApproverType string         `bson:"approverType" json:"approverType"`
	RoleID       *bson.ObjectID `bson:"roleId,omitempty" json:"roleId,omitempty"`
	UserID       *bson.ObjectID `bson:"userId,omitempty" json:"userId,omitempty"`
	// Fallback is true when the configured approver could not be resolved and
	// the step was routed to HR instead.
	Fallback  bool           `bson:"fallback,omitempty" json:"fallback,omitempty"`
	Decision  string         `bson:"decision" json:"decision"`
	DecidedBy *bson.ObjectID `bson:"decidedBy,omitempty" json:"decidedBy,omitempty"`
	DecidedAt *time.Time     `bson:"decidedAt,omitempty" json:"decidedAt,omitempty"`
	Comment   string         `bson:"comment,omitempty" json:"comment,omitempty"`
}

// AssignedTo reports whether the given user (with the given role) is the approver for this step.
func (s ApprovalStep) AssignedTo(userID, roleID bson.ObjectID) bool {
	if s.UserID != nil {
		return *s.UserID == userID
	}
	return s.RoleID != nil && *s.RoleID == roleID
}

type StatusChange struct {
	Status string         `bson:"status" json:"status"`
	At     time.Time      `bson:"at" json:"at"`
	By     *bson.ObjectID `bson:"by,omitempty" json:"by,omitempty"`
	Note   string         `bson:"note,omitempty" json:"note,omitempty"`
}

type Separation struct {
	ID                      bson.ObjectID  `bson:"_id" json:"id"`
	OrganizationID          bson.ObjectID  `bson:"organizationId" json:"-"`
	EmployeeID              bson.ObjectID  `bson:"employeeId" json:"employeeId"`
	Type                    string         `bson:"type" json:"type"`
	ReasonCode              string         `bson:"reasonCode" json:"reasonCode"`
	Category                string         `bson:"category" json:"category"`
	ReasonDetails           string         `bson:"reasonDetails,omitempty" json:"reasonDetails,omitempty"`
	SubmittedBy             bson.ObjectID  `bson:"submittedBy" json:"submittedBy"`
	SubmissionDate          Date           `bson:"submissionDate" json:"submissionDate"`
	RequestedLastWorkingDay Date           `bson:"requestedLastWorkingDay" json:"requestedLastWorkingDay"`
	LastWorkingDay          Date           `bson:"lastWorkingDay" json:"lastWorkingDay"`
	Status                  string         `bson:"status" json:"status"`
	ApprovalSteps           []ApprovalStep `bson:"approvalSteps" json:"approvalSteps"`
	StatusHistory           []StatusChange `bson:"statusHistory" json:"statusHistory"`
	// PendingUserID or PendingRoleID mirrors the approver of the pending step.
	PendingUserID *bson.ObjectID `bson:"pendingUserId,omitempty" json:"-"`
	PendingRoleID *bson.ObjectID `bson:"pendingRoleId,omitempty" json:"-"`
	// Captured at completion so reports stay stable if the employee record changes later.
	DepartmentIDAtExit *bson.ObjectID `bson:"departmentIdAtExit,omitempty" json:"departmentIdAtExit,omitempty"`
	TenureDaysAtExit   *int           `bson:"tenureDaysAtExit,omitempty" json:"tenureDaysAtExit,omitempty"`
	// ImportBatchID marks a past separation created by a CSV import.
	ImportBatchID *bson.ObjectID `bson:"importBatchId,omitempty" json:"-"`
	Version       int            `bson:"version" json:"version"`
	CreatedAt     time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time      `bson:"updatedAt" json:"updatedAt"`
}

var (
	ErrInvalidTransition = errors.New("this action is not allowed in the separation's current status")
	ErrNotYetLastDay     = errors.New("the last working day has not been reached")
)

func (s *Separation) IsOpen() bool { return slices.Contains(OpenSeparationStatuses, s.Status) }

// PendingStep returns the step currently awaiting a decision, or nil.
func (s *Separation) PendingStep() *ApprovalStep {
	if s.Status != SeparationPending {
		return nil
	}
	for i := range s.ApprovalSteps {
		if s.ApprovalSteps[i].Decision == DecisionPending {
			return &s.ApprovalSteps[i]
		}
	}
	return nil
}

func (s *Separation) record(status string, by *bson.ObjectID, note string, now time.Time) {
	s.Status = status
	s.StatusHistory = append(s.StatusHistory, StatusChange{Status: status, At: now, By: by, Note: note})
	s.syncPending()
}

// syncPending refreshes the denormalised pending approver fields.
func (s *Separation) syncPending() {
	s.PendingUserID, s.PendingRoleID = nil, nil
	if step := s.PendingStep(); step != nil {
		if step.UserID != nil {
			s.PendingUserID = step.UserID
		} else {
			s.PendingRoleID = step.RoleID
		}
	}
}

// Start sets the initial status: pending when there are approval steps,
// approved straight away when there are none.
func (s *Separation) Start(by bson.ObjectID, now time.Time) {
	if len(s.ApprovalSteps) > 0 {
		s.record(SeparationPending, &by, "", now)
		return
	}
	s.record(SeparationApproved, &by, "", now)
}

// Approve records an approval of the pending step. It reports whether that
// was the final step, which makes the whole separation approved.
func (s *Separation) Approve(by bson.ObjectID, comment string, now time.Time) (final bool, err error) {
	step := s.PendingStep()
	if step == nil {
		return false, ErrInvalidTransition
	}
	step.Decision, step.DecidedBy, step.DecidedAt, step.Comment = DecisionApproved, &by, &now, comment
	if s.PendingStep() != nil {
		s.syncPending()
		return false, nil
	}
	s.record(SeparationApproved, &by, "", now)
	return true, nil
}

func (s *Separation) Reject(by bson.ObjectID, comment string, now time.Time) error {
	step := s.PendingStep()
	if step == nil {
		return ErrInvalidTransition
	}
	step.Decision, step.DecidedBy, step.DecidedAt, step.Comment = DecisionRejected, &by, &now, comment
	s.record(SeparationRejected, &by, comment, now)
	return nil
}

// Withdraw cancels a separation that is pending or approved but not completed.
func (s *Separation) Withdraw(by bson.ObjectID, note string, now time.Time) error {
	if !s.IsOpen() {
		return ErrInvalidTransition
	}
	s.record(SeparationWithdrawn, &by, note, now)
	return nil
}

// Complete closes an approved separation once the last working day has been
// reached, capturing the department and tenure at exit.
func (s *Separation) Complete(by bson.ObjectID, e *Employee, today Date, now time.Time) error {
	if s.Status != SeparationApproved {
		return ErrInvalidTransition
	}
	if today.Before(s.LastWorkingDay) {
		return ErrNotYetLastDay
	}
	dept := e.DepartmentID
	tenure := int(s.LastWorkingDay.Sub(e.JoinDate.Time).Hours() / 24)
	s.DepartmentIDAtExit, s.TenureDaysAtExit = &dept, &tenure
	s.record(SeparationCompleted, &by, "", now)
	return nil
}
