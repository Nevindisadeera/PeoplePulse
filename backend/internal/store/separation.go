package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

// Atomic runs fn in a transaction when the deployment supports one (a replica
// set). On a standalone MongoDB it runs fn directly, so multi-document changes
// are then not atomic; use a replica set outside local development.
func (s *Store) Atomic(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.Transactions {
		return s.WithTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Store) InsertSeparation(ctx context.Context, sep *domain.Separation) error {
	_, err := s.DB.Collection("separations").InsertOne(ctx, sep)
	return mapErr(err)
}

func (s *Store) SeparationByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.Separation, error) {
	var sep domain.Separation
	err := s.DB.Collection("separations").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&sep)
	return &sep, mapErr(err)
}

// SaveSeparation writes the mutable workflow fields if the version still matches.
func (s *Store) SaveSeparation(ctx context.Context, sep *domain.Separation, version int) error {
	set := bson.M{
		"status": sep.Status, "approvalSteps": sep.ApprovalSteps, "statusHistory": sep.StatusHistory,
		"lastWorkingDay": sep.LastWorkingDay,
	}
	if sep.DepartmentIDAtExit != nil {
		set["departmentIdAtExit"] = sep.DepartmentIDAtExit
		set["tenureDaysAtExit"] = sep.TenureDaysAtExit
	}
	// The pending approver is denormalised so "awaiting me" is a simple indexed query.
	unset := []string{}
	for field, id := range map[string]*bson.ObjectID{"pendingUserId": sep.PendingUserID, "pendingRoleId": sep.PendingRoleID} {
		if id != nil {
			set[field] = *id
		} else {
			unset = append(unset, field)
		}
	}
	return s.updateVersioned(ctx, "separations", sep.OrganizationID, sep.ID, version, set, unset)
}

type SeparationFilter struct {
	Status     string
	EmployeeID *bson.ObjectID
	// Visibility, when set, limits results to separations the caller is involved in.
	Visibility *SeparationVisibility
	// AwaitingUser/AwaitingRole limit results to requests whose pending step is assigned to them.
	AwaitingUser *bson.ObjectID
	AwaitingRole *bson.ObjectID
}

type SeparationVisibility struct {
	EmployeeID *bson.ObjectID // the caller's own employee record
	UserID     bson.ObjectID
	RoleID     *bson.ObjectID // set when the caller may approve
}

func (s *Store) ListSeparations(ctx context.Context, orgID bson.ObjectID, f SeparationFilter, page, pageSize int) ([]domain.Separation, int64, error) {
	and := bson.A{bson.M{"organizationId": orgID}}
	if f.Status != "" {
		and = append(and, bson.M{"status": f.Status})
	}
	if f.EmployeeID != nil {
		and = append(and, bson.M{"employeeId": *f.EmployeeID})
	}
	if v := f.Visibility; v != nil {
		or := bson.A{bson.M{"submittedBy": v.UserID}, bson.M{"approvalSteps.userId": v.UserID}}
		if v.EmployeeID != nil {
			or = append(or, bson.M{"employeeId": *v.EmployeeID})
		}
		if v.RoleID != nil {
			or = append(or, bson.M{"approvalSteps.roleId": *v.RoleID})
		}
		and = append(and, bson.M{"$or": or})
	}
	if f.AwaitingUser != nil {
		or := bson.A{bson.M{"pendingUserId": *f.AwaitingUser}}
		if f.AwaitingRole != nil {
			or = append(or, bson.M{"pendingRoleId": *f.AwaitingRole})
		}
		and = append(and, bson.M{"status": domain.SeparationPending}, bson.M{"$or": or})
	}
	filter := bson.M{"$and": and}
	total, err := s.DB.Collection("separations").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("separations").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	out := []domain.Separation{}
	return out, total, cur.All(ctx, &out)
}

// PatchEmployee applies a system-driven change (status, end date) without a
// caller-supplied version, still bumping the version so open edit forms go stale.
func (s *Store) PatchEmployee(ctx context.Context, orgID, id bson.ObjectID, set bson.M, unset ...string) error {
	set["updatedAt"] = time.Now().UTC()
	update := bson.M{"$set": set, "$inc": bson.M{"version": 1}}
	if len(unset) > 0 {
		u := bson.M{}
		for _, f := range unset {
			u[f] = ""
		}
		update["$unset"] = u
	}
	res, err := s.DB.Collection("employees").UpdateOne(ctx, bson.M{"_id": id, "organizationId": orgID}, update)
	if err != nil {
		return mapErr(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) EmployeesByIDs(ctx context.Context, orgID bson.ObjectID, ids []bson.ObjectID) (map[bson.ObjectID]domain.Employee, error) {
	out := map[bson.ObjectID]domain.Employee{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.DB.Collection("employees").Find(ctx, bson.M{"organizationId": orgID, "_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	var list []domain.Employee
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	for _, e := range list {
		out[e.ID] = e
	}
	return out, nil
}

// ActiveUserByEmployeeID finds the active user account linked to an employee record.
func (s *Store) ActiveUserByEmployeeID(ctx context.Context, orgID, employeeID bson.ObjectID) (*domain.User, error) {
	var u domain.User
	err := s.DB.Collection("users").FindOne(ctx,
		bson.M{"organizationId": orgID, "employeeId": employeeID, "status": domain.UserStatusActive}).Decode(&u)
	return &u, mapErr(err)
}

func (s *Store) RoleByName(ctx context.Context, orgID bson.ObjectID, name string) (*domain.Role, error) {
	var r domain.Role
	err := s.DB.Collection("roles").FindOne(ctx, bson.M{"organizationId": orgID, "name": name}).Decode(&r)
	return &r, mapErr(err)
}

func (s *Store) LockedRole(ctx context.Context, orgID bson.ObjectID) (*domain.Role, error) {
	var r domain.Role
	err := s.DB.Collection("roles").FindOne(ctx, bson.M{"organizationId": orgID, "locked": true}).Decode(&r)
	return &r, mapErr(err)
}

func (s *Store) CountSeparationsWithReason(ctx context.Context, orgID bson.ObjectID, code string) (int64, error) {
	return s.DB.Collection("separations").CountDocuments(ctx, bson.M{"organizationId": orgID, "reasonCode": code})
}
