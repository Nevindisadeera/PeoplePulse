package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"peoplepulse/internal/domain"
)

func (s *Store) InsertExitInterview(ctx context.Context, iv *domain.ExitInterview) error {
	_, err := s.DB.Collection("exit_interviews").InsertOne(ctx, iv)
	return mapErr(err)
}

func (s *Store) ExitInterviewByID(ctx context.Context, orgID, id bson.ObjectID) (*domain.ExitInterview, error) {
	var iv domain.ExitInterview
	err := s.DB.Collection("exit_interviews").FindOne(ctx, bson.M{"_id": id, "organizationId": orgID}).Decode(&iv)
	return &iv, mapErr(err)
}

func (s *Store) ExitInterviewBySeparation(ctx context.Context, orgID, separationID bson.ObjectID) (*domain.ExitInterview, error) {
	var iv domain.ExitInterview
	err := s.DB.Collection("exit_interviews").FindOne(ctx, bson.M{"organizationId": orgID, "separationId": separationID}).Decode(&iv)
	return &iv, mapErr(err)
}

// SaveExitInterview writes the editable fields if the version still matches.
func (s *Store) SaveExitInterview(ctx context.Context, iv *domain.ExitInterview, version int) error {
	set := bson.M{"answers": iv.Answers, "notes": iv.Notes, "status": iv.Status}
	unset := []string{}
	optional := func(field string, present bool, value any) {
		if present {
			set[field] = value
		} else {
			unset = append(unset, field)
		}
	}
	optional("interviewerId", iv.InterviewerID != nil, iv.InterviewerID)
	optional("scheduledAt", iv.ScheduledAt != nil, iv.ScheduledAt)
	optional("submittedAt", iv.SubmittedAt != nil, iv.SubmittedAt)
	optional("submittedBy", iv.SubmittedBy != nil, iv.SubmittedBy)
	return s.updateVersioned(ctx, "exit_interviews", iv.OrganizationID, iv.ID, version, set, unset)
}

func (s *Store) ListExitInterviews(ctx context.Context, orgID bson.ObjectID, status string, page, pageSize int) ([]domain.ExitInterview, int64, error) {
	filter := bson.M{"organizationId": orgID}
	if status != "" {
		filter["status"] = status
	}
	total, err := s.DB.Collection("exit_interviews").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := s.DB.Collection("exit_interviews").Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	out := []domain.ExitInterview{}
	return out, total, cur.All(ctx, &out)
}

// SubmittedExitInterviews returns interviews submitted in [from, to).
func (s *Store) SubmittedExitInterviews(ctx context.Context, orgID bson.ObjectID, from, to time.Time) ([]domain.ExitInterview, error) {
	cur, err := s.DB.Collection("exit_interviews").Find(ctx, bson.M{
		"organizationId": orgID, "status": domain.InterviewSubmitted,
		"submittedAt": bson.M{"$gte": from, "$lt": to},
	}, options.Find().SetProjection(bson.M{"questions": 1, "answers": 1}))
	if err != nil {
		return nil, err
	}
	out := []domain.ExitInterview{}
	return out, cur.All(ctx, &out)
}
