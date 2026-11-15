package domain

import (
	"strings"
	"testing"
)

func intp(n int) *int { return &n }

var testQuestions = []InterviewQuestion{
	{ID: "sat", Text: "Satisfaction", Type: QuestionRating, Required: true},
	{ID: "rec", Text: "Recommend", Type: QuestionChoice, Options: []string{"Yes", "No"}, Required: true},
	{ID: "why", Text: "Comments", Type: QuestionText},
}

func TestCleanAnswers(t *testing.T) {
	// A draft may be partial; blanks are dropped and order follows the questions.
	clean, problems := CleanAnswers(testQuestions, []InterviewAnswer{
		{QuestionID: "why", Text: "Long commute"}, {QuestionID: "sat", Rating: intp(4)}, {QuestionID: "rec"},
	}, false)
	if len(problems) != 0 || len(clean) != 2 || clean[0].QuestionID != "sat" || clean[1].QuestionID != "why" {
		t.Errorf("draft: clean=%+v problems=%v", clean, problems)
	}

	// On submission the same answers are missing a required question.
	if _, problems = CleanAnswers(testQuestions, clean, true); problems["rec"] != "is required" || len(problems) != 1 {
		t.Errorf("submit with missing required answer: %v", problems)
	}

	_, problems = CleanAnswers(testQuestions, []InterviewAnswer{
		{QuestionID: "sat", Rating: intp(6)},
		{QuestionID: "rec", Choice: "Perhaps"},
		{QuestionID: "why", Text: strings.Repeat("x", MaxAnswerTextLength+1)},
		{QuestionID: "ghost", Text: "?"},
	}, false)
	for _, id := range []string{"sat", "rec", "why", "ghost"} {
		if problems[id] == "" {
			t.Errorf("expected a problem for %q, got %v", id, problems)
		}
	}

	// The wrong kind of value for the question type is rejected.
	_, problems = CleanAnswers(testQuestions, []InterviewAnswer{{QuestionID: "sat", Text: "five"}, {QuestionID: "why", Rating: intp(3)}}, false)
	if problems["sat"] == "" || problems["why"] == "" {
		t.Errorf("wrong value kinds accepted: %v", problems)
	}

	clean, problems = CleanAnswers(testQuestions, []InterviewAnswer{{QuestionID: "sat", Rating: intp(1)}, {QuestionID: "rec", Choice: "No"}}, true)
	if len(problems) != 0 || len(clean) != 2 {
		t.Errorf("complete submission: clean=%+v problems=%v", clean, problems)
	}
}

func interview(rating int, choice, text string) ExitInterview {
	iv := ExitInterview{Questions: testQuestions, Answers: []InterviewAnswer{{QuestionID: "sat", Rating: intp(rating)}, {QuestionID: "rec", Choice: choice}}}
	if text != "" {
		iv.Answers = append(iv.Answers, InterviewAnswer{QuestionID: "why", Text: text})
	}
	return iv
}

func TestAggregateFeedback(t *testing.T) {
	list := []ExitInterview{interview(5, "Yes", "secret one"), interview(4, "Yes", ""), interview(2, "No", "secret two")}

	// Below the threshold nothing at all is returned.
	if s := AggregateFeedback(list, 5); !s.Suppressed || len(s.Questions) != 0 || s.Interviews != 3 {
		t.Errorf("below threshold: %+v", s)
	}

	s := AggregateFeedback(list, 3)
	if s.Suppressed || len(s.Questions) != 3 {
		t.Fatalf("at threshold: %+v", s)
	}
	sat, rec, why := s.Questions[0], s.Questions[1], s.Questions[2]
	// (5 + 4 + 2) / 3 = 3.666..., rounded to 3.67.
	if sat.Average == nil || *sat.Average != 3.67 || sat.Distribution["5"] != 1 || sat.Distribution["2"] != 1 || sat.Responses != 3 {
		t.Errorf("rating summary: avg=%v %+v", sat.Average, sat)
	}
	if rec.Distribution["Yes"] != 2 || rec.Distribution["No"] != 1 || rec.Average != nil {
		t.Errorf("choice summary: %+v", rec)
	}
	// Only two of three answered the text question: suppressed, and text never appears.
	if !why.Suppressed || why.Responses != 2 || why.Distribution != nil {
		t.Errorf("text summary: %+v", why)
	}
}
