package api

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

const (
	maxImportBytes  = 2 << 20
	maxImportRows   = 5000
	maxImportErrors = 200
)

// Columns: the five in importRequired are mandatory. workEmail, phone,
// jobTitle and managerEmployeeNumber are optional. For someone who has
// already left, endDate and separationReason are given too, and
// separationType may be.
var importRequired = []string{"employeeNumber", "fullName", "departmentCode", "employmentType", "joinDate"}

const importedNote = "Imported from CSV. The submission date was not recorded and is set to the last working day."

type importError struct {
	Row     int    `json:"row"` // 1 is the header row
	Field   string `json:"field"`
	Message string `json:"message"`
}

type importLookups struct {
	departments map[string]bson.ObjectID // upper-case code -> id, active only
	types       map[string]bool          // active employment type codes
	existing    map[string]store.EmployeeRef
	reasons     map[string]domain.SeparationReason // every configured reason, by code
	importer    bson.ObjectID                      // the user running the import
	today       domain.Date
}

type importReport struct {
	DryRun    bool `json:"dryRun"`
	TotalRows int  `json:"totalRows"`
	ValidRows int  `json:"validRows"`
	Imported  int  `json:"imported"`
	// Leavers is how many of the rows are people who have already left.
	Leavers int           `json:"leavers"`
	Errors  []importError `json:"errors"`
}

// parseEmployeeCSV validates every row and builds the employee records, plus a
// completed separation for each person who has already left, so that they
// count in headcount and turnover for the time they were employed. It never
// stops at the first problem, so the user gets one complete error list.
func parseEmployeeCSV(r io.Reader, lk importLookups, orgID, batchID bson.ObjectID, now time.Time) (emps []domain.Employee, seps []domain.Separation, total int, errs []importError) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, nil, 0, []importError{{Row: 1, Field: "file", Message: "the file is empty or is not valid CSV"}}
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	for _, name := range importRequired {
		if _, ok := col[strings.ToLower(name)]; !ok {
			errs = append(errs, importError{Row: 1, Field: name, Message: "required column is missing from the header row"})
		}
	}
	if len(errs) > 0 {
		return nil, nil, 0, errs
	}

	type pending struct {
		row     int
		manager string
	}
	var managers []pending
	inFile := map[string]int{} // employee number -> index in emps
	badRows := map[int]bool{}
	fail := func(row int, field, msg string) {
		badRows[row] = true
		errs = append(errs, importError{Row: row, Field: field, Message: msg})
	}

	for row := 2; ; row++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fail(row, "file", "this line is not valid CSV")
			break
		}
		get := func(name string) string {
			if i, ok := col[strings.ToLower(name)]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue // blank line
		}
		total++
		if total > maxImportRows {
			fail(row, "file", "the file has more than 5,000 rows; split it into smaller files")
			break
		}

		e := domain.Employee{
			ID: bson.NewObjectID(), OrganizationID: orgID, ImportBatchID: &batchID,
			EmployeeNumber: get("employeeNumber"), FullName: get("fullName"),
			WorkEmail: strings.ToLower(get("workEmail")), Phone: get("phone"), JobTitle: get("jobTitle"),
			EmploymentType: strings.ToLower(get("employmentType")), Status: domain.EmployeeActive,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		switch {
		case e.EmployeeNumber == "":
			fail(row, "employeeNumber", "is required")
		case len(e.EmployeeNumber) > 30:
			fail(row, "employeeNumber", "must be at most 30 characters")
		default:
			if _, dup := lk.existing[e.EmployeeNumber]; dup {
				fail(row, "employeeNumber", "is already used by an existing employee")
			} else if _, dup := inFile[e.EmployeeNumber]; dup {
				fail(row, "employeeNumber", "appears more than once in the file")
			}
		}
		if n := len(e.FullName); n < 2 || n > 120 {
			fail(row, "fullName", "must be 2 to 120 characters")
		}
		if e.WorkEmail != "" {
			if a, err := mail.ParseAddress(e.WorkEmail); err != nil || a.Address != e.WorkEmail {
				fail(row, "workEmail", "must be a valid email address")
			}
		}
		if len(e.Phone) > 30 {
			fail(row, "phone", "must be at most 30 characters")
		}
		if len(e.JobTitle) > 120 {
			fail(row, "jobTitle", "must be at most 120 characters")
		}
		if dept, ok := lk.departments[strings.ToUpper(get("departmentCode"))]; ok {
			e.DepartmentID = dept
		} else {
			fail(row, "departmentCode", "is not the code of an active department")
		}
		if !lk.types[e.EmploymentType] {
			fail(row, "employmentType", "is not an active employment type code")
		}
		if join, err := domain.ParseDate(get("joinDate")); err != nil {
			fail(row, "joinDate", err.Error())
		} else if join.After(lk.today.AddYears(1)) {
			fail(row, "joinDate", "cannot be more than one year in the future")
		} else {
			e.JoinDate = join
			e.DepartmentHistory = []domain.DepartmentAssignment{{DepartmentID: e.DepartmentID, From: join}}
		}

		// Someone who has already left: an end date with the reason they left.
		endRaw, reasonCode, sepType := get("endDate"), strings.ToLower(get("separationReason")), strings.ToLower(get("separationType"))
		if endRaw == "" {
			if reasonCode != "" || sepType != "" {
				fail(row, "endDate", "is required when a separation reason or type is given")
			}
		} else {
			ok := true
			bad := func(field, msg string) {
				ok = false
				fail(row, field, msg)
			}
			end, err := domain.ParseDate(endRaw)
			switch {
			case err != nil:
				bad("endDate", err.Error())
			case end.After(lk.today):
				bad("endDate", "cannot be in the future; import the person as a current employee and record the separation in the application")
			case !e.JoinDate.IsZero() && end.Before(e.JoinDate):
				bad("endDate", "cannot be before the join date")
			}
			reason, known := lk.reasons[reasonCode]
			switch {
			case reasonCode == "":
				bad("separationReason", "is required for someone who has left")
			case !known:
				bad("separationReason", "is not the code of a separation reason in this organization")
			}
			if known {
				if sepType == "" {
					// Without a type, a voluntary reason means a resignation.
					sepType = "other"
					if reason.Category == domain.CategoryVoluntary {
						sepType = domain.SeparationTypeResignation
					}
				}
				switch {
				case !slices.Contains(domain.SeparationTypes, sepType):
					bad("separationType", "must be one of: "+strings.Join(domain.SeparationTypes, ", "))
				case (sepType == domain.SeparationTypeResignation) != (reason.Category == domain.CategoryVoluntary):
					bad("separationType", "does not fit the reason: a resignation takes a voluntary reason, other types take an involuntary or other reason")
				}
			}
			if ok && !e.JoinDate.IsZero() && e.EmployeeNumber != "" {
				e.EndDate, e.Status = &end, domain.EmployeeSeparated
				e.DepartmentHistory[0].To = &end
				dept, tenure := e.DepartmentID, int(end.Sub(e.JoinDate.Time).Hours()/24)
				seps = append(seps, domain.Separation{
					ID: bson.NewObjectID(), OrganizationID: orgID, EmployeeID: e.ID, ImportBatchID: &batchID,
					Type: sepType, ReasonCode: reason.Code, Category: reason.Category, SubmittedBy: lk.importer,
					SubmissionDate: end, RequestedLastWorkingDay: end, LastWorkingDay: end, Status: domain.SeparationCompleted,
					ApprovalSteps:      []domain.ApprovalStep{},
					StatusHistory:      []domain.StatusChange{{Status: domain.SeparationCompleted, At: now, By: &lk.importer, Note: importedNote}},
					DepartmentIDAtExit: &dept, TenureDaysAtExit: &tenure, Version: 1, CreatedAt: now, UpdatedAt: now,
				})
			}
		}

		if _, dup := inFile[e.EmployeeNumber]; !dup && e.EmployeeNumber != "" {
			inFile[e.EmployeeNumber] = len(emps)
		}
		managers = append(managers, pending{row: row, manager: get("managerEmployeeNumber")})
		emps = append(emps, e)
	}

	// Second pass: a manager may be an existing employee or another row in the file.
	for i, m := range managers {
		if m.manager == "" {
			continue
		}
		if m.manager == emps[i].EmployeeNumber {
			fail(m.row, "managerEmployeeNumber", "cannot be the employee's own number")
		} else if j, ok := inFile[m.manager]; ok && emps[j].Status == domain.EmployeeSeparated && emps[i].Status != domain.EmployeeSeparated {
			fail(m.row, "managerEmployeeNumber", "is someone who has left, and cannot manage a current employee")
		} else if ok {
			emps[i].ManagerID = &emps[j].ID
		} else if ref, ok := lk.existing[m.manager]; ok && ref.Status != domain.EmployeeSeparated {
			id := ref.ID
			emps[i].ManagerID = &id
		} else {
			fail(m.row, "managerEmployeeNumber", "does not match a current employee or another row in the file")
		}
	}
	// A separation is kept only if its row ended up valid.
	valid := seps[:0]
	bad := map[bson.ObjectID]bool{}
	for i, m := range managers {
		if badRows[m.row] {
			bad[emps[i].ID] = true
		}
	}
	for _, sep := range seps {
		if !bad[sep.EmployeeID] {
			valid = append(valid, sep)
		}
	}
	return emps, valid, total - len(badRows), errs
}

// handleImportEmployees accepts a CSV file (multipart field "file", or the raw
// request body). With ?dryRun=true it only validates. A real import saves
// nothing unless every row is valid.
func (s *Server) handleImportEmployees(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	dryRun := r.URL.Query().Get("dryRun") == "true"
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)

	var src io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest("Attach the CSV file in a form field named \"file\" (maximum 2 MB)."))
			return
		}
		defer file.Close()
		src = file
	}

	org, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lk := importLookups{departments: map[string]bson.ObjectID{}, types: map[string]bool{}, existing: map[string]store.EmployeeRef{}, reasons: map[string]domain.SeparationReason{}, importer: p.User.ID, today: today}
	// Any configured reason is accepted for a past separation, including one that has since been deactivated.
	for _, reason := range org.SeparationReasons() {
		lk.reasons[reason.Code] = reason
	}
	depts, err := s.store.ListDepartments(r.Context(), orgID, domain.DepartmentActive)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, d := range depts {
		lk.departments[d.Code] = d.ID
	}
	for _, t := range org.EmploymentTypes() {
		if t.Active {
			lk.types[t.Code] = true
		}
	}
	refs, err := s.store.EmployeeRefs(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, ref := range refs {
		lk.existing[ref.EmployeeNumber] = ref
	}

	batchID := bson.NewObjectID()
	emps, seps, valid, errs := parseEmployeeCSV(src, lk, orgID, batchID, time.Now().UTC())
	report := importReport{DryRun: dryRun, TotalRows: len(emps), ValidRows: valid, Leavers: len(seps), Errors: errs}
	if report.Errors == nil {
		report.Errors = []importError{}
	}
	if len(report.Errors) > maxImportErrors {
		report.Errors = report.Errors[:maxImportErrors]
	}
	if len(errs) > 0 && !dryRun {
		httpx.JSON(w, 422, map[string]any{
			"error":  map[string]any{"code": "IMPORT_INVALID", "message": "Nothing was imported. Fix the listed rows and upload the file again."},
			"report": report,
		})
		return
	}
	if dryRun || len(emps) == 0 {
		httpx.JSON(w, 200, report)
		return
	}
	// Employees and the separations of those who left are saved together or not at all.
	err = s.store.Atomic(r.Context(), func(ctx context.Context) error {
		if err := s.store.InsertEmployees(ctx, emps); err != nil {
			return err
		}
		return s.store.InsertSeparations(ctx, seps)
	})
	if err != nil {
		// Without a transaction, undo the rows that were written before the failure.
		_ = s.store.DeleteImportBatch(r.Context(), orgID, batchID)
		if errors.Is(err, store.ErrDuplicate) {
			err = httpx.Conflict("An employee number in the file was created by someone else during the import. Nothing was imported; try again.")
		}
		httpx.Fail(w, r, err)
		return
	}
	report.Imported = len(emps)
	s.audit(r, orgID, &p.User.ID, "employee.imported", "import", batchID.Hex(), map[string]any{"count": len(emps), "leavers": len(seps)})
	httpx.JSON(w, 201, report)
}
