package domain

// Permission is one entry of the fixed permission catalogue. Roles are named
// sets of these keys; organizations can edit roles but not the catalogue.
type Permission struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

const (
	PermOrgSettingsManage   = "org.settings.manage"
	PermUserRead            = "user.read"
	PermUserManage          = "user.manage"
	PermRoleRead            = "role.read"
	PermRoleManage          = "role.manage"
	PermEmployeeRead        = "employee.read"
	PermEmployeeReadDept    = "employee.read_department"
	PermEmployeeManage      = "employee.manage"
	PermDepartmentRead      = "department.read"
	PermDepartmentManage    = "department.manage"
	PermSeparationRead      = "separation.read"
	PermSeparationSubmit    = "separation.submit"
	PermSeparationManage    = "separation.manage"
	PermSeparationApprove   = "separation.approve"
	PermSeparationRecordInv = "separation.record_involuntary"
	PermClearanceRead       = "clearance.read"
	PermClearanceUpdateOwn  = "clearance.update_assigned"
	PermClearanceManage     = "clearance.manage"
	PermClearanceOverride   = "clearance.override"
	PermExitInterviewRead   = "exit_interview.read"
	PermExitInterviewManage = "exit_interview.manage"
	PermExitInterviewAmend  = "exit_interview.amend"
	PermAnalyticsRead       = "analytics.read"
	PermReportExport        = "report.export"
	PermAuditRead           = "audit.read"
)

var Catalogue = []Permission{
	{PermOrgSettingsManage, "Organization", "Edit organization profile and settings"},
	{PermUserRead, "Access", "View users"},
	{PermUserManage, "Access", "Invite, edit and deactivate users"},
	{PermRoleRead, "Access", "View roles and permissions"},
	{PermRoleManage, "Access", "Create and edit roles"},
	{PermEmployeeRead, "Workforce", "View all employee records"},
	{PermEmployeeReadDept, "Workforce", "View employees in departments I head"},
	{PermEmployeeManage, "Workforce", "Create, edit, import and transfer employees"},
	{PermDepartmentRead, "Workforce", "View departments"},
	{PermDepartmentManage, "Workforce", "Create, edit and archive departments"},
	{PermSeparationRead, "Exits", "View separations"},
	{PermSeparationSubmit, "Exits", "Submit own resignation"},
	{PermSeparationManage, "Exits", "Record and manage separations for any employee"},
	{PermSeparationApprove, "Exits", "Approve or reject assigned approval steps"},
	{PermSeparationRecordInv, "Exits", "Record involuntary separations"},
	{PermClearanceRead, "Exits", "View all clearance checklists"},
	{PermClearanceUpdateOwn, "Exits", "Update clearance items assigned to me"},
	{PermClearanceManage, "Exits", "Update any clearance item"},
	{PermClearanceOverride, "Exits", "Complete a separation with open clearance items"},
	{PermExitInterviewRead, "Exits", "Read exit interview responses"},
	{PermExitInterviewManage, "Exits", "Schedule and record exit interviews, edit the question template"},
	{PermExitInterviewAmend, "Exits", "Amend an exit interview after it was submitted"},
	{PermAnalyticsRead, "Insight", "View dashboard and analytics"},
	{PermReportExport, "Insight", "Run and export reports"},
	{PermAuditRead, "Oversight", "Read the audit log"},
}

func IsValidPermission(key string) bool {
	for _, p := range Catalogue {
		if p.Key == key {
			return true
		}
	}
	return false
}

const RoleOrgAdmin = "Organization Admin"

// DefaultRoles returns the roles seeded for every new organization.
func DefaultRoles() []Role {
	all := []string{}
	for _, p := range Catalogue {
		if p.Key != PermEmployeeReadDept { // redundant next to employee.read
			all = append(all, p.Key)
		}
	}
	hrManager := []string{
		PermUserRead, PermRoleRead, PermEmployeeRead, PermEmployeeManage, PermDepartmentRead, PermDepartmentManage,
		PermSeparationRead, PermSeparationManage, PermSeparationApprove, PermSeparationRecordInv,
		PermClearanceRead, PermClearanceManage, PermClearanceOverride, PermClearanceUpdateOwn,
		PermExitInterviewRead, PermExitInterviewManage, PermExitInterviewAmend, PermAnalyticsRead, PermReportExport, PermAuditRead,
	}
	hrOfficer := []string{
		PermEmployeeRead, PermEmployeeManage, PermDepartmentRead, PermSeparationRead, PermSeparationManage,
		PermClearanceRead, PermClearanceManage, PermClearanceUpdateOwn,
		PermExitInterviewRead, PermExitInterviewManage, PermAnalyticsRead, PermReportExport,
	}
	return []Role{
		{Name: RoleOrgAdmin, Description: "Organization settings, users, roles and audit log.", Permissions: all, IsSystemDefault: true, Locked: true},
		{Name: "HR Manager", Description: "All HR data, final approvals, overrides and analytics.", Permissions: hrManager, IsSystemDefault: true},
		{Name: "HR Officer", Description: "Day-to-day employee records, separations and clearance follow-up.", Permissions: hrOfficer, IsSystemDefault: true},
		{Name: "Department Manager", Description: "Own department, approvals for direct reports, handover items.", Permissions: []string{PermEmployeeReadDept, PermDepartmentRead, PermSeparationSubmit, PermSeparationApprove, PermClearanceUpdateOwn, PermAnalyticsRead}, IsSystemDefault: true},
		{Name: "Clearance Officer", Description: "Clearance items assigned to them.", Permissions: []string{PermSeparationSubmit, PermClearanceUpdateOwn}, IsSystemDefault: true},
		{Name: "Employee", Description: "Own resignation and exit interview form.", Permissions: []string{PermSeparationSubmit}, IsSystemDefault: true},
	}
}
