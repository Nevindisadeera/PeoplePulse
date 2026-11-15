package outbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/store"
)

// fakeQueue holds one event and records what the dispatcher did with it.
type fakeQueue struct {
	event     *domain.OutboxEvent
	delivered bool
	failed    bool
	nextAt    time.Time
	lastError string
}

func (q *fakeQueue) ClaimOutboxEvent(_ context.Context, now time.Time, _ time.Duration) (*domain.OutboxEvent, error) {
	if q.event == nil || q.delivered || q.failed || q.nextAt.After(now) {
		return nil, store.ErrNotFound
	}
	q.event.Attempts++
	e := *q.event
	return &e, nil
}

func (q *fakeQueue) MarkOutboxDelivered(context.Context, bson.ObjectID, time.Time) error {
	q.delivered = true
	return nil
}

func (q *fakeQueue) MarkOutboxAttemptFailed(_ context.Context, _ bson.ObjectID, next time.Time, reason string, failed bool) error {
	q.nextAt, q.lastError, q.failed = next, reason, failed
	return nil
}

func newEvent() *domain.OutboxEvent {
	return &domain.OutboxEvent{ID: bson.NewObjectID(), EventID: "evt_1", OrganizationID: bson.NewObjectID(),
		Type: domain.EventSeparationApproved, Payload: bson.M{"title": "Approved"}, CreatedAt: time.Now()}
}

func TestDeliverySignsTheExactBody(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	var gotValid bool
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts := r.Header.Get(HeaderTimestamp)
		// What a receiver does: recompute the signature and check the timestamp is recent.
		sec, _ := strconv.ParseInt(ts, 10, 64)
		gotValid = r.Header.Get(HeaderSignature) == Sign(secret, ts, body) &&
			time.Since(time.Unix(sec, 0)) < 5*time.Minute && r.Header.Get(HeaderEventID) == "evt_1"
		_ = json.Unmarshal(body, &gotBody)
	}))
	defer srv.Close()

	q := &fakeQueue{event: newEvent()}
	d := New(q, srv.URL, secret)
	if !d.DeliverNext(context.Background()) {
		t.Fatal("expected an event to be processed")
	}
	if !gotValid {
		t.Error("receiver could not verify the signature, timestamp or event ID")
	}
	if !q.delivered || gotBody["type"] != domain.EventSeparationApproved || gotBody["eventId"] != "evt_1" {
		t.Errorf("delivered=%v body=%v", q.delivered, gotBody)
	}
	if d.DeliverNext(context.Background()) {
		t.Error("a delivered event was processed again")
	}
	if Sign(secret, "1", []byte("a")) == Sign("another-secret-another-secret-xx", "1", []byte("a")) ||
		Sign(secret, "1", []byte("a")) == Sign(secret, "2", []byte("a")) {
		t.Error("signature does not depend on the secret and the timestamp")
	}
}

func TestFailedDeliveryIsRetriedThenAbandoned(t *testing.T) {
	calls, healthy := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if !healthy {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	q := &fakeQueue{event: newEvent()}
	d := New(q, srv.URL, "0123456789abcdef0123456789abcdef")
	d.Now = func() time.Time { return now }

	d.DeliverNext(context.Background())
	if q.delivered || q.failed || q.lastError != "webhook responded 502" || !q.nextAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("after first failure: delivered=%v failed=%v err=%q next=%v", q.delivered, q.failed, q.lastError, q.nextAt)
	}
	if d.DeliverNext(context.Background()) {
		t.Error("event was retried before its backoff elapsed")
	}

	// The webhook recovers: the retry succeeds once the backoff has passed.
	healthy, now = true, now.Add(31*time.Second)
	d.DeliverNext(context.Background())
	if !q.delivered || calls != 2 {
		t.Errorf("after recovery: delivered=%v calls=%d", q.delivered, calls)
	}

	// An event that keeps failing is abandoned at the attempt limit.
	healthy = false
	q = &fakeQueue{event: newEvent()}
	d.Queue = q
	q.event.Attempts = MaxAttempts - 1
	d.DeliverNext(context.Background())
	if !q.failed {
		t.Error("event was not abandoned at the attempt limit")
	}
}

func TestBackoff(t *testing.T) {
	for attempts, want := range map[int]time.Duration{1: 30 * time.Second, 2: time.Minute, 4: 4 * time.Minute, 8: time.Hour, 60: time.Hour, 0: 30 * time.Second} {
		if got := Backoff(attempts); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempts, got, want)
		}
	}
}
