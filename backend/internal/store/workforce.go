package store

import (
	"context"
	"errors"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

// ErrVersion means the record exists but was changed since the caller read it.
var ErrVersion = errors.New("version conflict")

// updateVersioned applies an update only if the stored version matches, and
// increments the version. It distinguishes a stale version from a missing record.
func (s *Store) updateVersioned(ctx context.Context, coll string, orgID, id bson.ObjectID, version int, set bson.M, unset []string) error {
	set["updatedAt"] = time.Now().UTC()
	update := bson.M{"$set": set, "$inc": bson.M{"version": 1}}
	if len(unset) > 0 {
		u := bson.M{}
		for _, f := range unset {
			u[f] = ""
		}
		update["$unset"] = u
	}
	res, err := s.DB.Collection(coll).UpdateOne(ctx, bson.M{"_id": id, "organizationId": orgID, "version": version}, update)
	if err != nil {
		return mapErr(err)
	}
	if res.MatchedCount == 1 {
		return nil
	}
	n, err := s.DB.Collection(coll).CountDocuments(ctx, bson.M{"_id": id, "organizationId": orgID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return ErrVersion
}

// ---- organization settings ----

func (s *Store) UpdateOrganization(ctx context.Context, id bson.ObjectID, set bson.M, unset ...string) error {
	if set == nil {
		set = bson.M{}
	}
	set["updatedAt"] = time.Now().UTC()
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		u := bson.M{}
		for _, f := range unset {
			u[f] = ""
		}
		update["$unset"] = u
	}
	res, err := s.DB.Collection("organizations").UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return mapErr(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- departments ----

func (s *Store) InsertDepartment(ctx context.Context, d *domain.Department) error {
	_, err := s.DB.Collection("departments").InsertOne(ctx, d)
	return mapErr(err)
}

func (s *Store) DepartmentByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.Department, error) {
	var d domain.Department
	err := s.DB.Collection("departments").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&d)
	return &d, mapErr(err)
}

// ListDepartments returns the organization's departments; status "" means all.
func (s *Store) ListDepartments(ctx context.Context, orgID bson.ObjectID, status string) ([]domain.Department, error) {
	filter := bson.M{"organizationId": orgID}
	if status != "" {
		filter["status"] = status
	}
	cur, err := s.DB.Collection("departments").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	out := []domain.Department{}
	return out, cur.All(ctx, &out)
}

func (s *Store) UpdateDepartment(ctx context.Context, orgID, id bson.ObjectID, version int, set bson.M, unset ...string) error {
	return s.updateVersioned(ctx, "departments", orgID, id, version, set, unset)
}

func (s *Store) CountActiveChildDepartments(ctx context.Context, orgID, parentID bson.ObjectID) (int64, error) {
	return s.DB.Collection("departments").CountDocuments(ctx,
		bson.M{"organizationId": orgID, "parentId": parentID, "status": domain.DepartmentActive})
}

// DepartmentIDsHeadedBy lists the departments whose head is the given employee.
func (s *Store) DepartmentIDsHeadedBy(ctx context.Context, orgID, employeeID bson.ObjectID) ([]bson.ObjectID, error) {
	cur, err := s.DB.Collection("departments").Find(ctx,
		bson.M{"organizationId": orgID, "headEmployeeId": employeeID}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]bson.ObjectID, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids, nil
}

// HeadcountByDepartment counts employees who have not separated, per department.
func (s *Store) HeadcountByDepartment(ctx context.Context, orgID bson.ObjectID) (map[bson.ObjectID]int64, error) {
	cur, err := s.DB.Collection("employees").Aggregate(ctx, bson.A{
		bson.M{"$match": bson.M{"organizationId": orgID, "status": bson.M{"$ne": domain.EmployeeSeparated}}},
		bson.M{"$group": bson.M{"_id": "$departmentId", "n": bson.M{"$sum": 1}}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID bson.ObjectID `bson:"_id"`
		N  int64         `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := map[bson.ObjectID]int64{}
	for _, r := range rows {
		out[r.ID] = r.N
	}
	return out, nil
}

// ---- employees ----

func (s *Store) InsertEmployee(ctx context.Context, e *domain.Employee) error {
	_, err := s.DB.Collection("employees").InsertOne(ctx, e)
	return mapErr(err)
}

func (s *Store) InsertEmployees(ctx context.Context, list []domain.Employee) error {
	_, err := s.DB.Collection("employees").InsertMany(ctx, list)
	return mapErr(err)
}

func (s *Store) DeleteImportBatch(ctx context.Context, orgID, batchID bson.ObjectID) error {
	_, err := s.DB.Collection("employees").DeleteMany(ctx, bson.M{"organizationId": orgID, "importBatchId": batchID})
	return err
}

func (s *Store) EmployeeByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.Employee, error) {
	var e domain.Employee
	err := s.DB.Collection("employees").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&e)
	return &e, mapErr(err)
}

// EmployeeRef is the minimum needed to resolve references during an import.
type EmployeeRef struct {
	ID             bson.ObjectID `bson:"_id"`
	EmployeeNumber string        `bson:"employeeNumber"`
	Status         string        `bson:"status"`
}

func (s *Store) EmployeeRefs(ctx context.Context, orgID bson.ObjectID) ([]EmployeeRef, error) {
	cur, err := s.DB.Collection("employees").Find(ctx, bson.M{"organizationId": orgID},
		options.Find().SetProjection(bson.M{"employeeNumber": 1, "status": 1}))
	if err != nil {
		return nil, err
	}
	var out []EmployeeRef
	return out, cur.All(ctx, &out)
}

type EmployeeFilter struct {
	Query         string
	Status        string
	DepartmentIDs []bson.ObjectID // nil means no department restriction
}

func (s *Store) ListEmployees(ctx context.Context, orgID bson.ObjectID, f EmployeeFilter, page, pageSize int) ([]domain.Employee, int64, error) {
	filter := bson.M{"organizationId": orgID}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.DepartmentIDs != nil {
		filter["departmentId"] = bson.M{"$in": f.DepartmentIDs}
	}
	if f.Query != "" {
		rx := bson.M{"$regex": regexp.QuoteMeta(f.Query), "$options": "i"}
		filter["$or"] = bson.A{bson.M{"fullName": rx}, bson.M{"employeeNumber": rx}, bson.M{"workEmail": rx}}
	}
	total, err := s.DB.Collection("employees").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("employees").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "fullName", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	out := []domain.Employee{}
	return out, total, cur.All(ctx, &out)
}

func (s *Store) UpdateEmployee(ctx context.Context, orgID, id bson.ObjectID, version int, set bson.M, unset ...string) error {
	return s.updateVersioned(ctx, "employees", orgID, id, version, set, unset)
}

func (s *Store) CountCurrentEmployeesInDepartment(ctx context.Context, orgID, deptID bson.ObjectID) (int64, error) {
	return s.DB.Collection("employees").CountDocuments(ctx,
		bson.M{"organizationId": orgID, "departmentId": deptID, "status": bson.M{"$ne": domain.EmployeeSeparated}})
}

func (s *Store) CountEmployeesWithType(ctx context.Context, orgID bson.ObjectID, code string) (int64, error) {
	return s.DB.Collection("employees").CountDocuments(ctx, bson.M{"organizationId": orgID, "employmentType": code})
}
