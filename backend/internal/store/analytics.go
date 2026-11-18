package store

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/metrics"
)

// MetricPeople loads every employment period whose employment type counts
// toward headcount, reduced to the fields the calculations need.
func (s *Store) MetricPeople(ctx context.Context, orgID bson.ObjectID, countedTypes []string) ([]metrics.Person, error) {
	cur, err := s.DB.Collection("employees").Find(ctx,
		bson.M{"organizationId": orgID, "employmentType": bson.M{"$in": countedTypes}},
		options.Find().SetProjection(bson.M{"joinDate": 1, "endDate": 1, "departmentHistory": 1}))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID      bson.ObjectID                 `bson:"_id"`
		Join    domain.Date                   `bson:"joinDate"`
		End     *domain.Date                  `bson:"endDate"`
		History []domain.DepartmentAssignment `bson:"departmentHistory"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]metrics.Person, len(rows))
	for i, r := range rows {
		out[i] = metrics.Person{ID: r.ID, Join: r.Join, End: r.End, Assignments: r.History}
	}
	return out, nil
}

// CompletedExits loads every completed separation as a metrics exit.
func (s *Store) CompletedExits(ctx context.Context, orgID bson.ObjectID) ([]metrics.Exit, error) {
	cur, err := s.DB.Collection("separations").Find(ctx,
		bson.M{"organizationId": orgID, "status": domain.SeparationCompleted},
		options.Find().SetProjection(bson.M{"employeeId": 1, "lastWorkingDay": 1, "category": 1, "reasonCode": 1, "departmentIdAtExit": 1, "tenureDaysAtExit": 1}))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		EmployeeID bson.ObjectID  `bson:"employeeId"`
		Date       domain.Date    `bson:"lastWorkingDay"`
		Category   string         `bson:"category"`
		Reason     string         `bson:"reasonCode"`
		Department *bson.ObjectID `bson:"departmentIdAtExit"`
		Tenure     *int           `bson:"tenureDaysAtExit"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]metrics.Exit, len(rows))
	for i, r := range rows {
		out[i] = metrics.Exit{EmployeeID: r.EmployeeID, Date: r.Date, Category: r.Category, ReasonCode: r.Reason}
		if r.Department != nil {
			out[i].Department = *r.Department
		}
		if r.Tenure != nil {
			out[i].TenureDays = *r.Tenure
		}
	}
	return out, nil
}

// QualityRow is an employee record reduced to the fields checked for data quality.
type QualityRow struct {
	ID             bson.ObjectID                 `bson:"_id"`
	EmployeeNumber string                        `bson:"employeeNumber"`
	Status         string                        `bson:"status"`
	Join           domain.Date                   `bson:"joinDate"`
	End            *domain.Date                  `bson:"endDate"`
	History        []domain.DepartmentAssignment `bson:"departmentHistory"`
}

func (s *Store) EmployeeQualityRows(ctx context.Context, orgID bson.ObjectID) ([]QualityRow, error) {
	cur, err := s.DB.Collection("employees").Find(ctx, bson.M{"organizationId": orgID},
		options.Find().SetProjection(bson.M{"employeeNumber": 1, "status": 1, "joinDate": 1, "endDate": 1, "departmentHistory": 1}))
	if err != nil {
		return nil, err
	}
	var out []QualityRow
	return out, cur.All(ctx, &out)
}
