package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

// ---- organizations ----

func (s *Store) InsertOrganization(ctx context.Context, o *domain.Organization) error {
	_, err := s.DB.Collection("organizations").InsertOne(ctx, o)
	return mapErr(err)
}

func (s *Store) OrganizationByID(ctx context.Context, id bson.ObjectID) (*domain.Organization, error) {
	var o domain.Organization
	err := s.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": id}).Decode(&o)
	return &o, mapErr(err)
}

// ---- roles ----

func (s *Store) InsertRole(ctx context.Context, r *domain.Role) error {
	_, err := s.DB.Collection("roles").InsertOne(ctx, r)
	return mapErr(err)
}

func (s *Store) RoleByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.Role, error) {
	var r domain.Role
	err := s.DB.Collection("roles").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&r)
	return &r, mapErr(err)
}

func (s *Store) ListRoles(ctx context.Context, orgID bson.ObjectID) ([]domain.Role, error) {
	cur, err := s.DB.Collection("roles").Find(ctx, bson.M{"organizationId": orgID},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	roles := []domain.Role{}
	return roles, cur.All(ctx, &roles)
}

func (s *Store) UpdateRole(ctx context.Context, orgID, id bson.ObjectID, set bson.M) error {
	set["updatedAt"] = time.Now().UTC()
	res, err := s.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id, "organizationId": orgID}, bson.M{"$set": set})
	if err != nil {
		return mapErr(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- users ----

func (s *Store) InsertUser(ctx context.Context, u *domain.User) error {
	_, err := s.DB.Collection("users").InsertOne(ctx, u)
	return mapErr(err)
}

// UserByEmail is the only unscoped user lookup; it is used for sign-in, where
// the organization is not yet known. Email is unique across the system.
func (s *Store) UserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := s.DB.Collection("users").FindOne(ctx, bson.M{"email": email}).Decode(&u)
	return &u, mapErr(err)
}

func (s *Store) UserByInviteHash(ctx context.Context, hash string) (*domain.User, error) {
	var u domain.User
	err := s.DB.Collection("users").FindOne(ctx, bson.M{"inviteTokenHash": hash}).Decode(&u)
	return &u, mapErr(err)
}

func (s *Store) UserByResetHash(ctx context.Context, hash string) (*domain.User, error) {
	var u domain.User
	err := s.DB.Collection("users").FindOne(ctx, bson.M{"resetTokenHash": hash}).Decode(&u)
	return &u, mapErr(err)
}

func (s *Store) UserByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.User, error) {
	var u domain.User
	err := s.DB.Collection("users").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&u)
	return &u, mapErr(err)
}

func (s *Store) ListUsers(ctx context.Context, orgID bson.ObjectID, page, pageSize int) ([]domain.User, int64, error) {
	filter := bson.M{"organizationId": orgID}
	total, err := s.DB.Collection("users").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("users").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "fullName", Value: 1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	users := []domain.User{}
	return users, total, cur.All(ctx, &users)
}

// UpdateUser applies $set and optional $unset to one user of the organization.
func (s *Store) UpdateUser(ctx context.Context, orgID, id bson.ObjectID, set bson.M, unset ...string) error {
	set["updatedAt"] = time.Now().UTC()
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		u := bson.M{}
		for _, f := range unset {
			u[f] = ""
		}
		update["$unset"] = u
	}
	res, err := s.DB.Collection("users").UpdateOne(ctx, bson.M{"_id": id, "organizationId": orgID}, update)
	if err != nil {
		return mapErr(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountActiveUsersWithRole(ctx context.Context, orgID, roleID bson.ObjectID) (int64, error) {
	return s.DB.Collection("users").CountDocuments(ctx,
		bson.M{"organizationId": orgID, "roleId": roleID, "status": domain.UserStatusActive})
}

func (s *Store) CountUsersWithRole(ctx context.Context, orgID, roleID bson.ObjectID) (int64, error) {
	return s.DB.Collection("users").CountDocuments(ctx, bson.M{"organizationId": orgID, "roleId": roleID})
}

// ---- sessions ----

func (s *Store) InsertSession(ctx context.Context, sess *domain.Session) error {
	_, err := s.DB.Collection("sessions").InsertOne(ctx, sess)
	return mapErr(err)
}

func (s *Store) SessionByHash(ctx context.Context, hash string) (*domain.Session, error) {
	var sess domain.Session
	err := s.DB.Collection("sessions").FindOne(ctx, bson.M{"tokenHash": hash}).Decode(&sess)
	return &sess, mapErr(err)
}

// MarkSessionRotated flags a refresh token as used. It returns false when the
// token had already been rotated (a concurrent or replayed refresh).
func (s *Store) MarkSessionRotated(ctx context.Context, id bson.ObjectID) (bool, error) {
	res, err := s.DB.Collection("sessions").UpdateOne(ctx,
		bson.M{"_id": id, "rotatedAt": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"rotatedAt": time.Now().UTC()}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

func (s *Store) DeleteSessionFamily(ctx context.Context, familyID bson.ObjectID) error {
	_, err := s.DB.Collection("sessions").DeleteMany(ctx, bson.M{"familyId": familyID})
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID bson.ObjectID) error {
	_, err := s.DB.Collection("sessions").DeleteMany(ctx, bson.M{"userId": userID})
	return err
}

// ---- audit ----

func (s *Store) InsertAudit(ctx context.Context, a *domain.AuditLog) error {
	_, err := s.DB.Collection("audit_logs").InsertOne(ctx, a)
	return err
}
