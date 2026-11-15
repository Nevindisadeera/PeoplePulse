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
		today: domain.NewDate(2026, 11, 15),
	}
}

func parse(csv string) ([]domain.Employee, int, []importError) {
	return parseEmployeeCSV(strings.NewReader(csv), testLookups(), bson.NewObjectID(), bson.NewObjectID(), time.Now())
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
