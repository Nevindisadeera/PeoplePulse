package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

var openItemStatuses = bson.A{domain.ItemPending, domain.ItemBlocked}

func (s *Store) InsertClearance(ctx context.Context, c *domain.ExitClearance) error {
	_, err := s.DB.Collection("exit_clearances").InsertOne(ctx, c)
	return mapErr(err)
}

func (s *Store) ClearanceByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.ExitClearance, error) {
	var c domain.ExitClearance
	err := s.DB.Collection("exit_clearances").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&c)
	return &c, mapErr(err)
}

func (s *Store) ClearanceBySeparation(ctx context.Context, orgID, separationID bson.ObjectID) (*domain.ExitClearance, error) {
	var c domain.ExitClearance
	err := s.DB.Collection("exit_clearances").FindOne(ctx, bson.M{"organizationId": orgID, "separationId": separationID}).Decode(&c)
	return &c, mapErr(err)
}

func (s *Store) ClearancesBySeparations(ctx context.Context, orgID bson.ObjectID, ids []bson.ObjectID) (map[bson.ObjectID]domain.ExitClearance, error) {
	out := map[bson.ObjectID]domain.ExitClearance{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.DB.Collection("exit_clearances").Find(ctx, bson.M{"organizationId": orgID, "separationId": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	var list []domain.ExitClearance
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	for _, c := range list {
		out[c.SeparationID] = c
	}
	return out, nil
}

// SaveClearance writes items, status and override if the version still matches.
func (s *Store) SaveClearance(ctx context.Context, c *domain.ExitClearance, version int) error {
	set := bson.M{"items": c.Items, "status": c.Status}
	if c.Override != nil {
		set["override"] = c.Override
	}
	return s.updateVersioned(ctx, "exit_clearances", c.OrganizationID, c.ID, version, set, nil)
}

// CancelClearance marks the checklist of a withdrawn separation as cancelled.
func (s *Store) CancelClearance(ctx context.Context, orgID, separationID bson.ObjectID) error {
	_, err := s.DB.Collection("exit_clearances").UpdateOne(ctx,
		bson.M{"organizationId": orgID, "separationId": separationID},
		bson.M{"$set": bson.M{"status": domain.ClearanceCancelled, "updatedAt": time.Now().UTC()}, "$inc": bson.M{"version": 1}})
	return err
}

// ShiftClearanceDueDates moves open items that were due on the old last
// working day to the new one. Items given their own due date are left alone.
func (s *Store) ShiftClearanceDueDates(ctx context.Context, orgID, separationID bson.ObjectID, from, to domain.Date) error {
	_, err := s.DB.Collection("exit_clearances").UpdateOne(ctx,
		bson.M{"organizationId": orgID, "separationId": separationID},
		bson.M{"$set": bson.M{"items.$[i].dueDate": to.Time, "updatedAt": time.Now().UTC()}, "$inc": bson.M{"version": 1}},
		options.UpdateOne().SetArrayFilters([]any{bson.M{"i.dueDate": from.Time, "i.status": bson.M{"$in": openItemStatuses}}}))
	return err
}

func (s *Store) ListClearances(ctx context.Context, orgID bson.ObjectID, status string, page, pageSize int) ([]domain.ExitClearance, int64, error) {
	filter := bson.M{"organizationId": orgID}
	if status != "" {
		filter["status"] = status
	}
	total, err := s.DB.Collection("exit_clearances").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("exit_clearances").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	out := []domain.ExitClearance{}
	return out, total, cur.All(ctx, &out)
}

// ClearancesWithOpenItemsFor returns in-progress checklists that have an open
// item assigned to the user directly or through their role.
func (s *Store) ClearancesWithOpenItemsFor(ctx context.Context, orgID, userID, roleID bson.ObjectID) ([]domain.ExitClearance, error) {
	cur, err := s.DB.Collection("exit_clearances").Find(ctx, bson.M{
		"organizationId": orgID, "status": domain.ClearanceInProgress,
		"items": bson.M{"$elemMatch": bson.M{
			"status": bson.M{"$in": openItemStatuses},
			"$or": bson.A{
				bson.M{"userId": userID},
				bson.M{"userId": bson.M{"$exists": false}, "roleId": roleID},
			},
		}},
	}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(500))
	if err != nil {
		return nil, err
	}
	out := []domain.ExitClearance{}
	return out, cur.All(ctx, &out)
}

func (s *Store) SeparationsByIDs(ctx context.Context, orgID bson.ObjectID, ids []bson.ObjectID) (map[bson.ObjectID]domain.Separation, error) {
	out := map[bson.ObjectID]domain.Separation{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.DB.Collection("separations").Find(ctx, bson.M{"organizationId": orgID, "_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	var list []domain.Separation
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	for _, x := range list {
		out[x.ID] = x
	}
	return out, nil
}
