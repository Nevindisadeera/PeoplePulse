package api

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/store"
)

func testLookups() importLookups {
	return importLookups{
		departments: map[string]bson.ObjectID{"ENG": bson.NewObjectID(), "HR": bson.NewObjectID()},
		types:       map[string]bool{"permanent": true},
		existing: map[string]store.EmployeeRef{
			"E001": {ID: bson.NewObjectID(), EmployeeNumber: "E001", Status: domain.EmployeeActive},
			"E002": {ID: bson.NewObjectID(), EmployeeNumber: "E002", Status: domain.EmployeeSeparated},
		},
		reasons: map[string]domain.SeparationReason{
			"personal":   {Code: "personal", Category: domain.CategoryVoluntary, Active: true},
			"misconduct": {Code: "misconduct", Category: domain.CategoryInvoluntary, Active: true},
			"retirement": {Code: "retirement", Category: domain.CategoryOther, Active: false},
		},
		today: domain.NewDate(2026, 11, 15),
	}
}

func parse(csv string) ([]domain.Employee, int, []importError) {
	emps, _, valid, errs := parseEmployeeCSV(strings.NewReader(csv), testLookups(), bson.NewObjectID(), bson.NewObjectID(), time.Now())
	return emps, valid, errs
}

func parseAll(csv string) ([]domain.Employee, []domain.Separation, []importError) {
	emps, seps, _, errs := parseEmployeeCSV(strings.NewReader(csv), testLookups(), bson.NewObjectID(), bson.NewObjectID(), time.Now())
	return emps, seps, errs
}

const leaverHeader = "employeeNumber,fullName,departmentCode,employmentType,joinDate,endDate,separationReason,separationType,managerEmployeeNumber\n"

func TestImportFormerEmployees(t *testing.T) {
	emps, seps, errs := parseAll(leaverHeader +
		"L1,Left Voluntarily,ENG,permanent,2024-01-10,2025-01-09,personal,,\n" + // type defaults to resignation
		"L2,Was Dismissed,ENG,permanent,2023-03-01,2023-08-31,misconduct,termination,L1\n" + // managed by another leaver: allowed
		"L3,Retired Person,HR,permanent,2020-01-01,2026-01-01,retirement,,\n" + // deactivated reason; type defaults to other
		"C1,Still Here,ENG,permanent,2025-02-01,,,,\n")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(emps) != 4 || len(seps) != 3 {
		t.Fatalf("got %d employees and %d separations, want 4 and 3", len(emps), len(seps))
	}
	left := emps[0]
	if left.Status != domain.EmployeeSeparated || left.EndDate.String() != "2025-01-09" || left.DepartmentHistory[0].To.String() != "2025-01-09" {
		t.Errorf("leaver record: status %s, end %v, history %+v", left.Status, left.EndDate, left.DepartmentHistory)
	}
	if emps[3].Status != domain.EmployeeActive || emps[3].EndDate != nil {
		t.Errorf("current employee was changed: %+v", emps[3])
	}
	// 2024-01-10 to 2025-01-09 spans the 2024 leap day: 365 days.
	s := seps[0]
	if s.EmployeeID != left.ID || s.Status != domain.SeparationCompleted || s.Type != domain.SeparationTypeResignation ||
		s.Category != domain.CategoryVoluntary || s.LastWorkingDay.String() != "2025-01-09" || *s.TenureDaysAtExit != 365 || *s.DepartmentIDAtExit != left.DepartmentID {
		t.Errorf("separation of the voluntary leaver: %+v", s)
	}
	if seps[1].Type != "termination" || seps[1].Category != domain.CategoryInvoluntary || seps[2].Type != "other" || seps[2].Category != domain.CategoryOther {
		t.Errorf("types and categories: %s/%s and %s/%s", seps[1].Type, seps[1].Category, seps[2].Type, seps[2].Category)
	}
	if s.ImportBatchID == nil || len(s.StatusHistory) != 1 {
		t.Errorf("an imported separation carries its batch and one history entry: %+v", s)
	}
}

func TestImportFormerEmployeeProblems(t *testing.T) {
	_, seps, errs := parseAll(leaverHeader +
		"P1,End Before Join,ENG,permanent,2025-01-10,2024-12-31,personal,,\n" + // row 2
		"P2,No Reason Given,ENG,permanent,2024-01-10,2025-01-09,,,\n" + // row 3
		"P3,Unknown Reason,ENG,permanent,2024-01-10,2025-01-09,boredom,,\n" + // row 4
		"P4,Wrong Type,ENG,permanent,2024-01-10,2025-01-09,misconduct,resignation,\n" + // row 5
		"P5,Future End,ENG,permanent,2024-01-10,2027-01-09,personal,,\n" + // row 6
		"P6,Reason No End,ENG,permanent,2024-01-10,,personal,,\n" + // row 7
		"P7,Current Staff,ENG,permanent,2024-01-10,,,,P8\n" + // row 8: managed by a leaver
		"P8,Fine Leaver,ENG,permanent,2024-01-10,2025-01-09,personal,,\n") // row 9: valid
	want := map[int]string{2: "endDate", 3: "separationReason", 4: "separationReason", 5: "separationType", 6: "endDate", 7: "endDate", 8: "managerEmployeeNumber"}
	got := map[int]string{}
	for _, e := range errs {
		got[e.Row] = e.Field
	}
	for row, field := range want {
		if got[row] != field {
			t.Errorf("row %d: error on %q, want %q (all: %+v)", row, got[row], field, errs)
		}
	}
	if len(got) != len(want) {
		t.Errorf("errors on rows %v, want only rows 2 to 8", got)
	}
	// Only the valid leaver produces a separation; rows with errors must not.
	if len(seps) != 1 {
		t.Errorf("got %d separations, want 1", len(seps))
	}
}

func TestImportValidFile(t *testing.T) {
	// Header has a BOM and different casing; E101 is managed by a later row, E102 by an existing employee.
	emps, valid, errs := parse("\ufeffEmployeeNumber,FullName,departmentCode,employmentType,joinDate,managerEmployeeNumber,workEmail\n" +
		"E101,Asha Perera,eng,Permanent,2025-01-06,E102,ASHA@example.com\n" +
		"\n" +
		"E102,Ravi Silva,HR,permanent,2023-07-01,E001,\n")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(emps) != 2 || valid != 2 {
		t.Fatalf("got %d employees, %d valid", len(emps), valid)
	}
	if emps[0].ManagerID == nil || *emps[0].ManagerID != emps[1].ID {
		t.Error("manager inside the file was not resolved")
	}
	if emps[1].ManagerID == nil {
		t.Error("existing manager was not resolved")
	}
	if emps[0].WorkEmail != "asha@example.com" || emps[0].EmploymentType != "permanent" {
		t.Errorf("values were not normalised: %+v", emps[0])
	}
	if h := emps[0].DepartmentHistory; len(h) != 1 || h[0].From.String() != "2025-01-06" || h[0].DepartmentID != emps[0].DepartmentID {
		t.Errorf("department history not initialised: %+v", h)
	}
}

func TestImportReportsEveryProblem(t *testing.T) {
	_, valid, errs := parse("employeeNumber,fullName,departmentCode,employmentType,joinDate,managerEmployeeNumber,workEmail\n" +
		"E001,Existing Number,ENG,permanent,2025-01-06,,\n" + // row 2: number exists
		"E201,X,NOPE,casual,06/01/2025,,not-an-email\n" + // row 3: five problems
		"E202,Dup One,ENG,permanent,2025-01-06,E002,\n" + // row 4: manager has separated
		"E202,Dup Two,ENG,permanent,2028-01-01,E202,\n" + // row 5: duplicate, future date, own manager
		"E203,Fine Person,ENG,permanent,2025-01-06,,\n")
	want := map[int][]string{
		2: {"employeeNumber"},
		3: {"fullName", "workEmail", "departmentCode", "employmentType", "joinDate"},
		4: {"managerEmployeeNumber"},
		5: {"employeeNumber", "joinDate", "managerEmployeeNumber"},
	}
	got := map[int]map[string]bool{}
	for _, e := range errs {
		if got[e.Row] == nil {
			got[e.Row] = map[string]bool{}
		}
		got[e.Row][e.Field] = true
	}
	for row, fields := range want {
		for _, f := range fields {
			if !got[row][f] {
				t.Errorf("row %d: missing error for %s (got %v)", row, f, got[row])
			}
		}
		if len(got[row]) != len(fields) {
			t.Errorf("row %d: got errors %v, want %v", row, got[row], fields)
		}
	}
	if len(got[6]) != 0 {
		t.Errorf("row 6 is valid but got errors %v", got[6])
	}
	if valid != 1 {
		t.Errorf("valid rows = %d, want 1", valid)
	}
}

func TestImportMissingColumnsAndEmptyFile(t *testing.T) {
	if _, _, errs := parse("employeeNumber,fullName\nE1,Name\n"); len(errs) != 3 {
		t.Errorf("want 3 missing-column errors, got %+v", errs)
	}
	if _, _, errs := parse(""); len(errs) != 1 {
		t.Errorf("want 1 error for an empty file, got %+v", errs)
	}
}
