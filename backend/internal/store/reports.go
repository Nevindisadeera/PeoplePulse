package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

// MaxReportRows caps how many records one report reads.
const MaxReportRows = 50000

type AuditFilter struct {
	Action     string
	EntityType string
	EntityID   string
	ActorID    *bson.ObjectID
	From, To   *time.Time // At in [From, To)
}

func (s *Store) ListAuditLogs(ctx context.Context, orgID bson.ObjectID, f AuditFilter, page, pageSize int) ([]domain.AuditLog, int64, error) {
	filter := bson.M{"organizationId": orgID}
	if f.Action != "" {
		filter["action"] = f.Action
	}
	if f.EntityType != "" {
		filter["entityType"] = f.EntityType
	}
	if f.EntityID != "" {
		filter["entityId"] = f.EntityID
	}
	if f.ActorID != nil {
		filter["actorId"] = *f.ActorID
	}
	at := bson.M{}
	if f.From != nil {
		at["$gte"] = *f.From
	}
	if f.To != nil {
		at["$lt"] = *f.To
	}
	if len(at) > 0 {
		filter["at"] = at
	}
	total, err := s.DB.Collection("audit_logs").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("audit_logs").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "at", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	out := []domain.AuditLog{}
	return out, total, cur.All(ctx, &out)
}

// UserNames maps user IDs to display names, including deactivated users.
func (s *Store) UserNames(ctx context.Context, orgID bson.ObjectID, ids []bson.ObjectID) (map[bson.ObjectID]string, error) {
	out := map[bson.ObjectID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.DB.Collection("users").Find(ctx, bson.M{"organizationId": orgID, "_id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"fullName": 1}))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID   bson.ObjectID `bson:"_id"`
		Name string        `bson:"fullName"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// ReportEmployees returns employees for the register, optionally limited to departments.
func (s *Store) ReportEmployees(ctx context.Context, orgID bson.ObjectID, departmentIDs []bson.ObjectID) ([]domain.Employee, error) {
	filter := bson.M{"organizationId": orgID}
	if departmentIDs != nil {
		filter["departmentId"] = bson.M{"$in": departmentIDs}
	}
	cur, err := s.DB.Collection("employees").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "employeeNumber", Value: 1}}).SetLimit(MaxReportRows))
	if err != nil {
		return nil, err
	}
	out := []domain.Employee{}
	return out, cur.All(ctx, &out)
}

// ReportSeparations returns separations whose last working day is in [from, to].
func (s *Store) ReportSeparations(ctx context.Context, orgID bson.ObjectID, from, to domain.Date) ([]domain.Separation, error) {
	cur, err := s.DB.Collection("separations").Find(ctx,
		bson.M{"organizationId": orgID, "lastWorkingDay": bson.M{"$gte": from.Time, "$lte": to.Time}},
		options.Find().SetSort(bson.D{{Key: "lastWorkingDay", Value: 1}}).SetLimit(MaxReportRows))
	if err != nil {
		return nil, err
	}
	out := []domain.Separation{}
	return out, cur.All(ctx, &out)
}

func (s *Store) ReportClearances(ctx context.Context, orgID bson.ObjectID) ([]domain.ExitClearance, error) {
	cur, err := s.DB.Collection("exit_clearances").Find(ctx, bson.M{"organizationId": orgID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(MaxReportRows))
	if err != nil {
		return nil, err
	}
	out := []domain.ExitClearance{}
	return out, cur.All(ctx, &out)
}
