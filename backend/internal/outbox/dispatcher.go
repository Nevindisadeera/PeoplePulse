// Package outbox delivers stored business events to the automation webhook (n8n).
package outbox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/store"
)

const (
	HeaderEventID   = "X-PeoplePulse-Event-Id"
	HeaderTimestamp = "X-PeoplePulse-Timestamp"
	HeaderSignature = "X-PeoplePulse-Signature"

	MaxAttempts = 10
	lease       = 2 * time.Minute
)

// Queue is the storage the dispatcher needs.
type Queue interface {
	ClaimOutboxEvent(ctx context.Context, now time.Time, lease time.Duration) (*domain.OutboxEvent, error)
	MarkOutboxDelivered(ctx context.Context, id bson.ObjectID, at time.Time) error
	MarkOutboxAttemptFailed(ctx context.Context, id bson.ObjectID, next time.Time, reason string, failed bool) error
}

type Dispatcher struct {
	Queue      Queue
	WebhookURL string
	Secret     string
	Client     *http.Client
	Poll       time.Duration
	Now        func() time.Time
}

func New(q Queue, url, secret string) *Dispatcher {
	return &Dispatcher{Queue: q, WebhookURL: url, Secret: secret,
		Client: &http.Client{Timeout: 10 * time.Second}, Poll: 5 * time.Second, Now: time.Now}
}

// Sign returns the hex HMAC-SHA256 of "<timestamp>.<body>". The receiver
// recomputes it with the shared secret and rejects mismatches and old timestamps.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Backoff is the wait before the next attempt: 30s, 1m, 2m, ... capped at 1 hour.
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := 30 * time.Second << (attempts - 1)
	if d > time.Hour || d <= 0 {
		return time.Hour
	}
	return d
}

// Run delivers due events until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.Poll)
	defer ticker.Stop()
	for {
		for d.DeliverNext(ctx) {
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// DeliverNext sends one due event. It reports whether an event was processed,
// so the caller can drain the queue before waiting.
func (d *Dispatcher) DeliverNext(ctx context.Context) bool {
	now := d.Now().UTC()
	ev, err := d.Queue.ClaimOutboxEvent(ctx, now, lease)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) && ctx.Err() == nil {
			slog.Error("outbox claim failed", "err", err)
		}
		return false
	}
	if err := d.send(ctx, ev, now); err != nil {
		failed := ev.Attempts >= MaxAttempts
		if merr := d.Queue.MarkOutboxAttemptFailed(ctx, ev.ID, now.Add(Backoff(ev.Attempts)), err.Error(), failed); merr != nil {
			slog.Error("outbox update failed", "eventId", ev.EventID, "err", merr)
		}
		slog.Warn("outbox delivery failed", "eventId", ev.EventID, "type", ev.Type, "attempt", ev.Attempts, "gaveUp", failed, "err", err)
		return true
	}
	if err := d.Queue.MarkOutboxDelivered(ctx, ev.ID, now); err != nil {
		// The event stays leased and will be sent again; receivers de-duplicate on the event ID.
		slog.Error("outbox update failed", "eventId", ev.EventID, "err", err)
	}
	return true
}

func (d *Dispatcher) send(ctx context.Context, ev *domain.OutboxEvent, now time.Time) error {
	body, err := json.Marshal(map[string]any{
		"eventId": ev.EventID, "type": ev.Type, "organizationId": ev.OrganizationID.Hex(),
		"occurredAt": ev.CreatedAt.UTC().Format(time.RFC3339), "data": ev.Payload,
	})
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderEventID, ev.EventID)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, Sign(d.Secret, ts, body))
	res, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("webhook responded %d", res.StatusCode)
	}
	return nil
}
