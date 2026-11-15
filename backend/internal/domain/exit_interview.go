package domain

import (
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	QuestionRating = "rating" // whole number 1 to 5
	QuestionChoice = "choice" // one of the question's options
	QuestionText   = "text"   // free text

	InterviewDraft     = "draft"
	InterviewSubmitted = "submitted"

	MaxAnswerTextLength          = 2000
	DefaultMinAggregateResponses = 5
)

type InterviewQuestion struct {
	ID       string   `bson:"id" json:"id"`
	Text     string   `bson:"text" json:"text"`
	Type     string   `bson:"type" json:"type"`
	Options  []string `bson:"options,omitempty" json:"options,omitempty"`
	Required bool     `bson:"required" json:"required"`
	Active   bool     `bson:"active" json:"active"`
}

// ExitInterviewTemplate is the organization's question set. Holding exit
// interviews is optional, so the whole feature can be switched off.
type ExitInterviewTemplate struct {
	Enabled bool `bson:"enabled" json:"enabled"`
	// EmployeeSelfService lets the departing employee fill in the form themselves.
	EmployeeSelfService bool `bson:"employeeSelfService" json:"employeeSelfService"`
	// MinAggregateResponses is the fewest submitted interviews for which
	// aggregated results are shown, to make individuals harder to identify.
	MinAggregateResponses int                 `bson:"minAggregateResponses" json:"minAggregateResponses"`
	Questions             []InterviewQuestion `bson:"questions" json:"questions"`
}

func DefaultExitInterviewTemplate() ExitInterviewTemplate {
	q := func(text, kind string, required bool, options ...string) InterviewQuestion {
		return InterviewQuestion{ID: bson.NewObjectID().Hex(), Text: text, Type: kind, Options: options, Required: required, Active: true}
	}
	return ExitInterviewTemplate{
		Enabled: true, EmployeeSelfService: true, MinAggregateResponses: DefaultMinAggregateResponses,
		Questions: []InterviewQuestion{
			q("Overall, how satisfied were you working here?", QuestionRating, true),
			q("How would you rate the support from your manager?", QuestionRating, true),
			q("How would you rate your opportunities to learn and grow?", QuestionRating, false),
			q("Would you recommend this organization as a place to work?", QuestionChoice, true, "Yes", "Maybe", "No"),
			q("What is the main thing we could have done better?", QuestionText, false),
		},
	}
}

type InterviewAnswer struct {
	QuestionID string `bson:"questionId" json:"questionId"`
	Rating     *int   `bson:"rating,omitempty" json:"rating,omitempty"`
	Choice     string `bson:"choice,omitempty" json:"choice,omitempty"`
	Text       string `bson:"text,omitempty" json:"text,omitempty"`
}

func (a InterviewAnswer) empty() bool { return a.Rating == nil && a.Choice == "" && a.Text == "" }

type ExitInterview struct {
	ID             bson.ObjectID  `bson:"_id" json:"id"`
	OrganizationID bson.ObjectID  `bson:"organizationId" json:"-"`
	SeparationID   bson.ObjectID  `bson:"separationId" json:"separationId"`
	EmployeeID     bson.ObjectID  `bson:"employeeId" json:"employeeId"`
	InterviewerID  *bson.ObjectID `bson:"interviewerId,omitempty" json:"interviewerId,omitempty"`
	ScheduledAt    *time.Time     `bson:"scheduledAt,omitempty" json:"scheduledAt,omitempty"`
	Status         string         `bson:"status" json:"status"`
	// Questions is a copy of the template taken when the interview was created.
	Questions   []InterviewQuestion `bson:"questions" json:"questions"`
	Answers     []InterviewAnswer   `bson:"answers" json:"answers"`
	Notes       string              `bson:"notes,omitempty" json:"notes,omitempty"`
	SubmittedAt *time.Time          `bson:"submittedAt,omitempty" json:"submittedAt,omitempty"`
	SubmittedBy *bson.ObjectID      `bson:"submittedBy,omitempty" json:"submittedBy,omitempty"`
	Version     int                 `bson:"version" json:"version"`
	CreatedAt   time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time           `bson:"updatedAt" json:"updatedAt"`
}

// CleanAnswers checks answers against the interview's questions and returns
// them in question order with blanks removed. With requireAll, every required
// question must be answered. Problems are keyed by question ID.
func CleanAnswers(questions []InterviewQuestion, answers []InterviewAnswer, requireAll bool) ([]InterviewAnswer, map[string]string) {
	problems := map[string]string{}
	byID := map[string]InterviewAnswer{}
	for _, a := range answers {
		if !slices.ContainsFunc(questions, func(q InterviewQuestion) bool { return q.ID == a.QuestionID }) {
			problems[a.QuestionID] = "is not a question in this interview"
			continue
		}
		if _, dup := byID[a.QuestionID]; dup {
			problems[a.QuestionID] = "is answered more than once"
			continue
		}
		byID[a.QuestionID] = a
	}
	clean := []InterviewAnswer{}
	for _, q := range questions {
		a, answered := byID[q.ID]
		out := InterviewAnswer{QuestionID: q.ID}
		switch {
		case !answered || a.empty():
			if requireAll && q.Required {
				problems[q.ID] = "is required"
			}
			continue
		case q.Type == QuestionRating:
			if a.Rating == nil || *a.Rating < 1 || *a.Rating > 5 || a.Choice != "" || a.Text != "" {
				problems[q.ID] = "must be a rating from 1 to 5"
				continue
			}
			out.Rating = a.Rating
		case q.Type == QuestionChoice:
			if a.Rating != nil || a.Text != "" || !slices.Contains(q.Options, a.Choice) {
				problems[q.ID] = "must be one of the listed options"
				continue
			}
			out.Choice = a.Choice
		default:
			if a.Rating != nil || a.Choice != "" {
				problems[q.ID] = "must be text"
				continue
			}
			if utf8.RuneCountInString(a.Text) > MaxAnswerTextLength {
				problems[q.ID] = fmt.Sprintf("must be at most %d characters", MaxAnswerTextLength)
				continue
			}
			out.Text = a.Text
		}
		clean = append(clean, out)
	}
	return clean, problems
}

// QuestionSummary is the aggregate of one question across interviews. Free
// text is counted but never included.
type QuestionSummary struct {
	QuestionID   string         `json:"questionId"`
	Text         string         `json:"text"`
	Type         string         `json:"type"`
	Responses    int            `json:"responses"`
	Suppressed   bool           `json:"suppressed"`
	Average      *float64       `json:"average,omitempty"`
	Distribution map[string]int `json:"distribution,omitempty"`
}

type FeedbackSummary struct {
	Interviews int  `json:"interviews"`
	Threshold  int  `json:"threshold"`
	Suppressed bool `json:"suppressed"`
	// Questions is empty when the whole summary is suppressed.
	Questions []QuestionSummary `json:"questions"`
}

// AggregateFeedback summarizes submitted interviews. Nothing is shown when
// there are fewer interviews than the threshold, and any single question with
// fewer answers than the threshold is suppressed on its own.
func AggregateFeedback(interviews []ExitInterview, threshold int) FeedbackSummary {
	if threshold < 1 {
		threshold = 1
	}
	out := FeedbackSummary{Interviews: len(interviews), Threshold: threshold, Questions: []QuestionSummary{}}
	if len(interviews) < threshold {
		out.Suppressed = true
		return out
	}
	index := map[string]int{}
	sums := map[string]int{}
	for _, iv := range interviews {
		for _, q := range iv.Questions {
			if _, seen := index[q.ID]; !seen {
				index[q.ID] = len(out.Questions)
				out.Questions = append(out.Questions, QuestionSummary{QuestionID: q.ID, Text: q.Text, Type: q.Type, Distribution: map[string]int{}})
			}
		}
		for _, a := range iv.Answers {
			i, ok := index[a.QuestionID]
			if !ok {
				continue
			}
			s := &out.Questions[i]
			s.Responses++
			switch s.Type {
			case QuestionRating:
				if a.Rating != nil {
					s.Distribution[fmt.Sprint(*a.Rating)]++
					sums[a.QuestionID] += *a.Rating
				}
			case QuestionChoice:
				s.Distribution[a.Choice]++
			}
		}
	}
	for i := range out.Questions {
		s := &out.Questions[i]
		switch {
		case s.Responses < threshold:
			s.Suppressed, s.Distribution = true, nil
		case s.Type == QuestionRating:
			avg := float64(int(float64(sums[s.QuestionID])/float64(s.Responses)*100+0.5)) / 100
			s.Average = &avg
		case s.Type == QuestionText:
			s.Distribution = nil
		}
	}
	return out
}
