package domain

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	DepartmentActive   = "active"
	DepartmentArchived = "archived"

	EmployeeActive        = "active"
	EmployeeServingNotice = "serving_notice"
	EmployeeSeparated     = "separated"
)

// EmploymentType is an organization-defined category such as permanent or
// intern. CountsInHeadcount decides whether it is included in metrics.
type EmploymentType struct {
	Code              string `bson:"code" json:"code"`
	Name              string `bson:"name" json:"name"`
	CountsInHeadcount bool   `bson:"countsInHeadcount" json:"countsInHeadcount"`
	Active            bool   `bson:"active" json:"active"`
}

type OrgSettings struct {
	EmploymentTypes   []EmploymentType        `bson:"employmentTypes" json:"employmentTypes"`
	NoticePeriodDays  *int                    `bson:"noticePeriodDays,omitempty" json:"noticePeriodDays"`
	SeparationReasons []SeparationReason      `bson:"separationReasons,omitempty" json:"separationReasons"`
	ApprovalChain     []ChainStep             `bson:"approvalChain,omitempty" json:"approvalChain"`
	ClearanceTemplate []ClearanceTemplateItem `bson:"clearanceTemplate,omitempty" json:"clearanceTemplate"`
	ExitInterview     *ExitInterviewTemplate  `bson:"exitInterview,omitempty" json:"exitInterview"`
	// AnalyticsStartDate is the earliest date from which the data is complete:
	// everyone employed on or after it has a record, including people who have
	// since left. No rate is stated for a period that starts before it.
	AnalyticsStartDate *Date `bson:"analyticsStartDate,omitempty" json:"analyticsStartDate"`
	// SmallGroupThreshold is the average headcount below which a rate is flagged.
	SmallGroupThreshold *int `bson:"smallGroupThreshold,omitempty" json:"smallGroupThreshold"`
	// ClearanceReminderDays: remind assignees about open clearance items this many days before they are due.
	ClearanceReminderDays *int `bson:"clearanceReminderDays,omitempty" json:"clearanceReminderDays"`
	// LastDayReminderDays: remind HR and the line manager this many days before a last working day.
	LastDayReminderDays *int `bson:"lastDayReminderDays,omitempty" json:"lastDayReminderDays"`
}

const (
	DefaultClearanceReminderDays = 3
	DefaultLastDayReminderDays   = 7
)

func (o *Organization) ClearanceReminderDays() int {
	if o.Settings.ClearanceReminderDays == nil {
		return DefaultClearanceReminderDays
	}
	return *o.Settings.ClearanceReminderDays
}

func (o *Organization) LastDayReminderDays() int {
	if o.Settings.LastDayReminderDays == nil {
		return DefaultLastDayReminderDays
	}
	return *o.Settings.LastDayReminderDays
}

const DefaultSmallGroupThreshold = 5

func (o *Organization) SmallGroupThreshold() int {
	if o.Settings.SmallGroupThreshold == nil {
		return DefaultSmallGroupThreshold
	}
	return *o.Settings.SmallGroupThreshold
}

// InterviewTemplate returns the exit interview settings; the feature is off
// until an organization has configured it.
func (o *Organization) InterviewTemplate() ExitInterviewTemplate {
	if o.Settings.ExitInterview == nil {
		return ExitInterviewTemplate{MinAggregateResponses: DefaultMinAggregateResponses, Questions: []InterviewQuestion{}}
	}
	t := *o.Settings.ExitInterview
	if t.Questions == nil {
		t.Questions = []InterviewQuestion{}
	}
	return t
}

const DefaultNoticePeriodDays = 30

// NoticePeriodDays returns the configured default notice period.
func (o *Organization) NoticePeriodDays() int {
	if o.Settings.NoticePeriodDays == nil {
		return DefaultNoticePeriodDays
	}
	return *o.Settings.NoticePeriodDays
}

// SeparationReasons returns the configured reasons, or the defaults when none are set.
func (o *Organization) SeparationReasons() []SeparationReason {
	if len(o.Settings.SeparationReasons) == 0 {
		return DefaultSeparationReasons()
	}
	return o.Settings.SeparationReasons
}

// Reason finds an active separation reason by code.
func (o *Organization) Reason(code string) (SeparationReason, bool) {
	for _, r := range o.SeparationReasons() {
		if r.Code == code && r.Active {
			return r, true
		}
	}
	return SeparationReason{}, false
}

func DefaultEmploymentTypes() []EmploymentType {
	return []EmploymentType{
		{Code: "permanent", Name: "Permanent", CountsInHeadcount: true, Active: true},
		{Code: "probation", Name: "Probation", CountsInHeadcount: true, Active: true},
		{Code: "contract", Name: "Fixed-term contract", CountsInHeadcount: true, Active: true},
		{Code: "intern", Name: "Intern", CountsInHeadcount: false, Active: true},
	}
}

// EmploymentTypes returns the configured types, or the defaults when none are set.
func (o *Organization) EmploymentTypes() []EmploymentType {
	if len(o.Settings.EmploymentTypes) == 0 {
		return DefaultEmploymentTypes()
	}
	return o.Settings.EmploymentTypes
}

// Location returns the organization's time zone, falling back to UTC.
func (o *Organization) Location() *time.Location {
	if loc, err := time.LoadLocation(o.TimeZone); err == nil {
		return loc
	}
	return time.UTC
}

type Department struct {
	ID             bson.ObjectID  `bson:"_id" json:"id"`
	OrganizationID bson.ObjectID  `bson:"organizationId" json:"-"`
	Name           string         `bson:"name" json:"name"`
	Code           string         `bson:"code" json:"code"`
	ParentID       *bson.ObjectID `bson:"parentId,omitempty" json:"parentId,omitempty"`
	HeadEmployeeID *bson.ObjectID `bson:"headEmployeeId,omitempty" json:"headEmployeeId,omitempty"`
	Status         string         `bson:"status" json:"status"`
	Version        int            `bson:"version" json:"version"`
	CreatedAt      time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time      `bson:"updatedAt" json:"updatedAt"`
}

// DepartmentAssignment is one dated period in a department. To is the last day
// in the department and is nil for the current assignment.
type DepartmentAssignment struct {
	DepartmentID bson.ObjectID `bson:"departmentId" json:"departmentId"`
	From         Date          `bson:"from" json:"from"`
	To           *Date         `bson:"to,omitempty" json:"to,omitempty"`
}

// Employee is one employment period. A rehired person gets a new record that
// points at the earlier one through RehireOf.
type Employee struct {
	ID                bson.ObjectID          `bson:"_id" json:"id"`
	OrganizationID    bson.ObjectID          `bson:"organizationId" json:"-"`
	EmployeeNumber    string                 `bson:"employeeNumber" json:"employeeNumber"`
	FullName          string                 `bson:"fullName" json:"fullName"`
	WorkEmail         string                 `bson:"workEmail,omitempty" json:"workEmail,omitempty"`
	Phone             string                 `bson:"phone,omitempty" json:"phone,omitempty"`
	DepartmentID      bson.ObjectID          `bson:"departmentId" json:"departmentId"`
	DepartmentHistory []DepartmentAssignment `bson:"departmentHistory" json:"departmentHistory"`
	JobTitle          string                 `bson:"jobTitle,omitempty" json:"jobTitle,omitempty"`
	EmploymentType    string                 `bson:"employmentType" json:"employmentType"`
	ManagerID         *bson.ObjectID         `bson:"managerId,omitempty" json:"managerId,omitempty"`
	JoinDate          Date                   `bson:"joinDate" json:"joinDate"`
	EndDate           *Date                  `bson:"endDate,omitempty" json:"endDate,omitempty"`
	Status            string                 `bson:"status" json:"status"`
	RehireOf          *bson.ObjectID         `bson:"rehireOf,omitempty" json:"rehireOf,omitempty"`
	ImportBatchID     *bson.ObjectID         `bson:"importBatchId,omitempty" json:"-"`
	Version           int                    `bson:"version" json:"version"`
	CreatedAt         time.Time              `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time              `bson:"updatedAt" json:"updatedAt"`
}

var (
	ErrSameDepartment   = errors.New("employee is already in this department")
	ErrTransferTooEarly = errors.New("effective date must be after the start of the current department assignment")
)

// ApplyTransfer closes the current assignment the day before the effective
// date and opens a new one. History is never overwritten.
func ApplyTransfer(history []DepartmentAssignment, to bson.ObjectID, effective Date) ([]DepartmentAssignment, error) {
	if len(history) == 0 {
		return []DepartmentAssignment{{DepartmentID: to, From: effective}}, nil
	}
	out := make([]DepartmentAssignment, len(history), len(history)+1)
	copy(out, history)
	last := &out[len(out)-1]
	if last.DepartmentID == to {
		return nil, ErrSameDepartment
	}
	if !effective.After(last.From) {
		return nil, ErrTransferTooEarly
	}
	end := effective.AddDays(-1)
	last.To = &end
	return append(out, DepartmentAssignment{DepartmentID: to, From: effective}), nil
}
