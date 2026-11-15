package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

// ---- notifications ----

func (s *Store) InsertNotifications(ctx context.Context, list []domain.Notification) error {
	if len(list) == 0 {
		return nil
	}
	_, err := s.DB.Collection("notifications").InsertMany(ctx, list)
	return mapErr(err)
}

func (s *Store) ListNotifications(ctx context.Context, orgID, userID bson.ObjectID, unreadOnly bool, page, pageSize int) ([]domain.Notification, int64, error) {
	filter := bson.M{"organizationId": orgID, "userId": userID}
	if unreadOnly {
		filter["readAt"] = bson.M{"$exists": false}
	}
	total, err := s.DB.Collection("notifications").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("notifications").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	out := []domain.Notification{}
	return out, total, cur.All(ctx, &out)
}

func (s *Store) CountUnreadNotifications(ctx context.Context, orgID, userID bson.ObjectID) (int64, error) {
	return s.DB.Collection("notifications").CountDocuments(ctx,
		bson.M{"organizationId": orgID, "userId": userID, "readAt": bson.M{"$exists": false}})
}

// MarkNotificationRead marks one of the user's own notifications as read.
func (s *Store) MarkNotificationRead(ctx context.Context, orgID, userID, id bson.ObjectID) error {
	res, err := s.DB.Collection("notifications").UpdateOne(ctx,
		bson.M{"_id": id, "organizationId": orgID, "userId": userID},
		bson.M{"$set": bson.M{"readAt": time.Now().UTC()}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context, orgID, userID bson.ObjectID) (int64, error) {
	res, err := s.DB.Collection("notifications").UpdateMany(ctx,
		bson.M{"organizationId": orgID, "userId": userID, "readAt": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"readAt": time.Now().UTC()}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

// ---- recipients ----

func (s *Store) ActiveUsersByIDs(ctx context.Context, orgID bson.ObjectID, ids []bson.ObjectID) ([]domain.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cur, err := s.DB.Collection("users").Find(ctx,
		bson.M{"organizationId": orgID, "_id": bson.M{"$in": ids}, "status": domain.UserStatusActive})
	if err != nil {
		return nil, err
	}
	var out []domain.User
	return out, cur.All(ctx, &out)
}

// ActiveUserIDsWithRoles returns the active users holding any of the roles.
func (s *Store) ActiveUserIDsWithRoles(ctx context.Context, orgID bson.ObjectID, roleIDs []bson.ObjectID) ([]bson.ObjectID, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	cur, err := s.DB.Collection("users").Find(ctx,
		bson.M{"organizationId": orgID, "roleId": bson.M{"$in": roleIDs}, "status": domain.UserStatusActive},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	return idsOf(ctx, cur)
}

// RoleIDsWithPermission returns the organization's roles that include a permission.
func (s *Store) RoleIDsWithPermission(ctx context.Context, orgID bson.ObjectID, perm string) ([]bson.ObjectID, error) {
	cur, err := s.DB.Collection("roles").Find(ctx, bson.M{"organizationId": orgID, "permissions": perm},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	return idsOf(ctx, cur)
}

type cursorAll interface {
	All(ctx context.Context, results any) error
}

func idsOf(ctx context.Context, cur cursorAll) ([]bson.ObjectID, error) {
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

// ---- outbox ----

func (s *Store) InsertOutboxEvent(ctx context.Context, e *domain.OutboxEvent) error {
	_, err := s.DB.Collection("outbox_events").InsertOne(ctx, e)
	return mapErr(err)
}

// ClaimOutboxEvent takes the next due event and leases it by pushing its next
// attempt into the future, so two API instances do not send it at the same
// time. It returns ErrNotFound when nothing is due.
func (s *Store) ClaimOutboxEvent(ctx context.Context, now time.Time, lease time.Duration) (*domain.OutboxEvent, error) {
	var e domain.OutboxEvent
	err := s.DB.Collection("outbox_events").FindOneAndUpdate(ctx,
		bson.M{"status": domain.OutboxPending, "nextAttemptAt": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"nextAttemptAt": now.Add(lease)}, "$inc": bson.M{"attempts": 1}},
		options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}}).SetReturnDocument(options.After),
	).Decode(&e)
	return &e, mapErr(err)
}

func (s *Store) MarkOutboxDelivered(ctx context.Context, id bson.ObjectID, at time.Time) error {
	_, err := s.DB.Collection("outbox_events").UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": domain.OutboxDelivered, "deliveredAt": at}, "$unset": bson.M{"lastError": ""}})
	return err
}

// MarkOutboxAttemptFailed schedules a retry, or gives up when failed is true.
func (s *Store) MarkOutboxAttemptFailed(ctx context.Context, id bson.ObjectID, next time.Time, reason string, failed bool) error {
	set := bson.M{"nextAttemptAt": next, "lastError": reason}
	if failed {
		set["status"] = domain.OutboxFailed
	}
	_, err := s.DB.Collection("outbox_events").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}
