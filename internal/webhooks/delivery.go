// Delivery execution: the in-process dispatcher used in single-process
// mode, the queue worker's re-run entry point, and the HMAC-signed POST
// both paths share.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// UseInlineDispatcher swaps the (nil) queue dispatcher for the in-process
// retry loop — the single-process development path.
func (s *Service) UseInlineDispatcher() {
	s.dispatcher = NewInlineDispatcher(s)
}

// InlineDispatcher runs deliveries in a background goroutine with
// three attempts and short backoff — the single-process development path.
type InlineDispatcher struct {
	svc *Service
}

// NewInlineDispatcher builds the in-process dispatcher.
func NewInlineDispatcher(svc *Service) *InlineDispatcher {
	return &InlineDispatcher{svc: svc}
}

// Deliver implements Dispatcher.
func (d *InlineDispatcher) Deliver(ctx context.Context, endpoint Endpoint, delivery Delivery, payload []byte) {
	go func() {
		bg := context.Background()
		d.svc.deliver(bg, endpoint, delivery, payload)
		for _, delay := range []time.Duration{2 * time.Second, 8 * time.Second} {
			if delivery.Status == "success" {
				return
			}
			time.Sleep(delay)
			d.svc.deliver(bg, endpoint, delivery, payload)
		}
	}()
}

// DeliverQueued re-runs one recorded delivery from the queue path: the
// worker loads endpoint and delivery by id and posts once.
func (s *Service) DeliverQueued(ctx context.Context, endpointID, deliveryID string) error {
	endpoint, err := s.store.GetWebhookEndpoint(ctx, endpointID)
	if err != nil {
		return err
	}
	deliveries, err := s.store.ListWebhookDeliveries(ctx, endpointID, 100, 0)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if delivery.ID != deliveryID {
			continue
		}
		s.deliver(ctx, endpoint, delivery, []byte(delivery.Payload))
		return nil
	}
	return kernel.ErrNotFound
}

// deliver posts the payload once and records the attempt. It is the body
// of one retry round for both the inline path and the queue worker.
func (s *Service) deliver(ctx context.Context, endpoint Endpoint, delivery Delivery, payload []byte) {
	if s.settings != nil && !s.settings.Bool(ctx, "webhooks.enabled") {
		return
	}
	body := payload
	if body == nil {
		body = []byte(delivery.Payload)
	}
	mac := hmac.New(sha256.New, []byte(endpoint.Secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	status, err := s.post(ctx, endpoint.URL, body, signature, delivery.ID)
	delivery.Attempts++
	delivery.LastResponseStatus = status
	switch {
	case err != nil:
		delivery.Status = "failed"
		delivery.LastError = err.Error()
	case status >= 200 && status < 300:
		delivery.Status = "success"
		now := time.Now()
		delivery.DeliveredAt = &now
	default:
		delivery.Status = "failed"
		delivery.LastError = fmt.Sprintf("endpoint answered %d", status)
	}
	if updateErr := s.store.UpdateWebhookDelivery(ctx, delivery); updateErr != nil {
		s.log("webhook delivery update failed", updateErr)
	}
}

func (s *Service) post(ctx context.Context, url string, body []byte, signature, deliveryID string) (int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Template-Signature", signature)
	httpReq.Header.Set("X-Template-Delivery", deliveryID)
	var status int
	// A slow endpoint can no longer stack unbounded goroutines: the
	// bulkhead admits APP_BULKHEAD_LIMIT concurrent deliveries and rejects
	// the excess fast (backpressure never counts as a breaker failure).
	// Inside it, a dead endpoint trips the webhooks.http breaker; while it
	// is open deliveries fail fast instead of queuing timeouts.
	err = s.bulkheads.Do(ctx, "webhooks.http", func() error {
		return s.breakers.Do(ctx, "webhooks.http", func() error {
			start := time.Now()
			resp, err := s.client.Do(httpReq)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			status = resp.StatusCode
			_ = time.Since(start)
			return nil
		})
	})
	if err != nil {
		return 0, err
	}
	return status, nil
}
