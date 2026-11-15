// Package store is the MongoDB data access layer. Every tenant-owned query
// takes the organization ID explicitly so no query can run unscoped.
package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
)

type Store struct {
	Client *mongo.Client
	DB     *mongo.Database
	// Transactions is true when the deployment is a replica set or sharded cluster.
	Transactions bool
}

func Connect(ctx context.Context, uri, dbName string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, err
	}
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	if err := client.Database("admin").RunCommand(pingCtx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return nil, err
	}
	return &Store{Client: client, DB: client.Database(dbName), Transactions: hello.SetName != "" || hello.Msg == "isdbgrid"}, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.Client.Ping(ctx, nil) }

func (s *Store) Close(ctx context.Context) error { return s.Client.Disconnect(ctx) }

// WithTx runs fn in a multi-document transaction (requires a replica set).
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	sess, err := s.Client.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}

func (s *Store) EnsureIndexes(ctx context.Context) error {
	// A fresh builder per index: the driver must not see one options value shared between models.
	unique := func() *options.IndexOptionsBuilder { return options.Index().SetUnique(true) }
	specs := map[string][]mongo.IndexModel{
		"users": {
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "roleId", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "inviteTokenHash", Value: 1}}, Options: options.Index().SetSparse(true)},
		},
		"roles": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "name", Value: 1}}, Options: unique()},
		},
		"sessions": {
			{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "userId", Value: 1}}},
			{Keys: bson.D{{Key: "familyId", Value: 1}}},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		},
		"departments": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "code", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "headEmployeeId", Value: 1}}},
		},
		"employees": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "employeeNumber", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "status", Value: 1}, {Key: "departmentId", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "joinDate", Value: 1}, {Key: "endDate", Value: 1}}},
		},
		"separations": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "status", Value: 1}, {Key: "lastWorkingDay", Value: 1}}},
			// At most one open separation per employee, enforced by the database.
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "employeeId", Value: 1}}, Options: unique().SetName("one_open_separation_per_employee").
				SetPartialFilterExpression(bson.M{"status": bson.M{"$in": bson.A{"pending_approval", "approved"}}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "employeeId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "pendingUserId", Value: 1}}, Options: options.Index().SetSparse(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "pendingRoleId", Value: 1}}, Options: options.Index().SetSparse(true)},
		},
		"exit_clearances": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "separationId", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "items.userId", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "items.roleId", Value: 1}}},
		},
		"exit_interviews": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "separationId", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "status", Value: 1}, {Key: "submittedAt", Value: 1}}},
		},
		"notifications": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "userId", Value: 1}, {Key: "readAt", Value: 1}, {Key: "createdAt", Value: -1}}},
			// Notifications are removed after 180 days.
			{Keys: bson.D{{Key: "createdAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(180 * 24 * 3600)},
		},
		"outbox_events": {
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "nextAttemptAt", Value: 1}}},
			{Keys: bson.D{{Key: "eventId", Value: 1}}, Options: unique()},
		},
		"audit_logs": {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "at", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "entityType", Value: 1}, {Key: "entityId", Value: 1}}},
		},
	}
	for coll, models := range specs {
		if _, err := s.DB.Collection(coll).Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mongo.ErrNoDocuments):
		return ErrNotFound
	case mongo.IsDuplicateKeyError(err):
		return ErrDuplicate
	}
	return err
}
