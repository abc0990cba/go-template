// External test package: the memory store (repository) implements both
// jobs.Store and analytics.ReportStore, mirroring production wiring.
package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/repository"
)

// queueFake records enqueued jobs; err fails every publish.
type queueFake struct {
	mu  sync.Mutex
	got []jobs.Job
	err error
}

func (q *queueFake) Enqueue(_ context.Context, job jobs.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.got = append(q.got, job)
	return nil
}

// observerFake records the metrics-side lifecycle callbacks.
type observerFake struct {
	mu       sync.Mutex
	queued   int
	finished []string
}

func (o *observerFake) JobQueued(context.Context) {
	o.mu.Lock()
	o.queued++
	o.mu.Unlock()
}

func (o *observerFake) JobFinished(_ context.Context, typ, status string, _ time.Duration) {
	o.mu.Lock()
	o.finished = append(o.finished, typ+"/"+status)
	o.mu.Unlock()
}

// sinkFake records emitted analytics event names.
type sinkFake struct {
	mu     sync.Mutex
	events []string
}

func (s *sinkFake) AddEvent(_ context.Context, name, _ string) {
	s.mu.Lock()
	s.events = append(s.events, name)
	s.mu.Unlock()
}

type fixture struct {
	svc      *jobs.Service
	store    *repository.MemoryStore
	observer *observerFake
	events   *sinkFake
	notify   []string
}

func newFixture(t *testing.T, queue *queueFake) *fixture {
	t.Helper()
	store := repository.NewMemoryStore()
	observer := &observerFake{}
	events := &sinkFake{}
	// A typed nil pointer would arrive as a non-nil interface and defeat
	// the service's nil-queue checks, so only set the port when given.
	var queuePort jobs.Queue
	if queue != nil {
		queuePort = queue
	}
	svc := jobs.NewService(store, store, queuePort, observer, events, &http.Client{}, nil, nil, 2, discardLogger())
	f := &fixture{svc: svc, store: store, observer: observer, events: events}
	svc.WithJobNotifier(func(job jobs.Job) {
		f.notify = append(f.notify, job.Status)
	})
	return f
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func listOpts() kernel.ListOptions { return kernel.ListOptions{Mode: "offset", Limit: 10} }

func TestEnqueueWithoutQueueRecordsAndKeepsLocal(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	job, err := f.svc.Enqueue(ctx, jobs.TypeReportGenerate, "{}", "u1")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if job.Status != "queued" || job.CreatedBy != "u1" {
		t.Errorf("queued record: want queued/u1, got %+v", job)
	}
	if f.observer.queued != 1 {
		t.Errorf("observer: want 1 JobQueued, got %d", f.observer.queued)
	}
	if len(f.notify) != 1 || f.notify[0] != "queued" {
		t.Errorf("notifier: want [queued], got %v", f.notify)
	}
	if len(f.events.events) != 1 || f.events.events[0] != "job_queued" {
		t.Errorf("events: want [job_queued], got %v", f.events.events)
	}

	// The job is listed back.
	listed, _, err := f.svc.List(ctx, listOpts())
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: want 1 job, got %d (%v)", len(listed), err)
	}
}

func TestEnqueueFailureMarksJobFailed(t *testing.T) {
	q := &queueFake{err: errors.New("redis down")}
	f := newFixture(t, q)
	ctx := context.Background()

	job, err := f.svc.Enqueue(ctx, jobs.TypeReportGenerate, "{}", "u1")
	if err == nil {
		t.Fatal("enqueue with failing queue: want error")
	}
	if !strings.Contains(err.Error(), "enqueue") {
		t.Errorf("enqueue error: want wrapped cause, got %v", err)
	}
	stored, err := f.svc.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Status != "failed" || !strings.Contains(stored.Error, "redis down") {
		t.Errorf("stored job: want failed with redis cause, got %+v", stored)
	}
	// The UI hears about both transitions.
	if len(f.notify) != 2 || f.notify[1] != "failed" {
		t.Errorf("notifier: want [queued failed], got %v", f.notify)
	}
}

func TestProcessOneRunsReportJobToCompletion(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	nots := notifications.NewService(f.store, nil)
	f.svc.WithNotifications(nots)

	if _, err := f.svc.Enqueue(ctx, jobs.TypeReportGenerate, "{}", "u1"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	processed, err := f.svc.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("process one: want (true, nil), got (%v, %v)", processed, err)
	}

	listed, _, err := f.svc.List(ctx, listOpts())
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: want 1 job, got %d (%v)", len(listed), err)
	}
	if listed[0].Status != "done" {
		t.Errorf("job: want done, got %+v", listed[0])
	}

	// The report itself was persisted.
	reports, _, err := f.store.ListReports(ctx, listOpts())
	if err != nil || len(reports) != 1 {
		t.Fatalf("reports: want 1, got %d (%v)", len(reports), err)
	}
	if reports[0].Status != "done" {
		t.Errorf("report: want done, got %+v", reports[0])
	}

	// The owner is told the job finished.
	userNotifications, err := nots.List(ctx, "u1", 10, 0)
	if err != nil || len(userNotifications) != 1 {
		t.Fatalf("owner notifications: want 1, got %d (%v)", len(userNotifications), err)
	}
	if !strings.Contains(userNotifications[0].Title, "finished") {
		t.Errorf("owner notification: want finished title, got %+v", userNotifications[0])
	}

	// Nothing is left to process.
	if processed, _ := f.svc.ProcessOne(ctx); processed {
		t.Error("second process one: want false")
	}
}

func TestProcessOneFailsUnknownAndUnwiredJobs(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	// Unknown types fail the job, not the processor.
	if _, err := f.svc.Enqueue(ctx, "no.such.type", "{}", "u1"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := f.svc.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	listed, _, _ := f.svc.List(ctx, listOpts())
	if listed[0].Status != "failed" || !strings.Contains(listed[0].Error, "unknown job type") {
		t.Errorf("unknown type: want failed with cause, got %+v", listed[0])
	}

	// The export job needs an exporter wired in this process.
	if _, err := f.svc.Enqueue(ctx, jobs.TypeExportGenerate, `{"user_id":"u1"}`, "u1"); err != nil {
		t.Fatalf("enqueue export: %v", err)
	}
	if _, err := f.svc.ProcessOne(ctx); err != nil {
		t.Fatalf("process export: %v", err)
	}
	listed, _, _ = f.svc.List(ctx, listOpts())
	if listed[0].Status != "failed" || !strings.Contains(listed[0].Error, "export builder") {
		t.Errorf("export without builder: want failed with cause, got %+v", listed[0])
	}
}

func TestEnrichBatchRejectsMalformedPayloadWithoutUpstreamCall(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	// Malformed JSON fails in the decoder, before any upstream request.
	if _, err := f.svc.Enqueue(ctx, jobs.TypeEnrichBatch, "not-json", "u1"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := f.svc.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	listed, _, _ := f.svc.List(ctx, listOpts())
	if listed[0].Status != "failed" || !strings.Contains(listed[0].Error, "decode payload") {
		t.Errorf("malformed payload: want failed decode, got %+v", listed[0])
	}
}

func TestProcessOneIsANoOpWithDistributedQueue(t *testing.T) {
	f := newFixture(t, &queueFake{})
	ctx := context.Background()
	if _, err := f.svc.Enqueue(ctx, jobs.TypeReportGenerate, "{}", "u1"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if processed, _ := f.svc.ProcessOne(ctx); processed {
		t.Error("process one with queue: want no-op false")
	}
}

// enrichmentFixture points the service's enrichment upstream at a stub
// server answering with the given status and body.
func enrichmentFixture(t *testing.T, status int, body string) *fixture {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	f := newFixture(t, nil)
	f.svc.WithEnrichmentURL(server.URL)
	return f
}

func runEnrichBatch(t *testing.T, f *fixture, payload string) jobs.Job {
	t.Helper()
	ctx := context.Background()
	if _, err := f.svc.Enqueue(ctx, jobs.TypeEnrichBatch, payload, "u1"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := f.svc.ProcessOne(ctx); err != nil {
		t.Fatalf("process one: %v", err)
	}
	listed, _, _ := f.svc.List(ctx, listOpts())
	return listed[0]
}

func TestEnrichBatchStoresUpstreamProfiles(t *testing.T) {
	f := enrichmentFixture(t, http.StatusOK, `{"results":[{"gender":"female"}]}`)
	job := runEnrichBatch(t, f, `{"count":2}`)

	if job.Status != "done" {
		t.Fatalf("batch: want done, got %+v", job)
	}
	profiles, _, err := f.store.ListEnrichments(context.Background(), listOpts())
	if err != nil || len(profiles) != 2 {
		t.Fatalf("enrichments: want 2, got %d (%v)", len(profiles), err)
	}
	for _, profile := range profiles {
		if profile.Source != "randomuser.me" {
			t.Errorf("source: want randomuser.me, got %q", profile.Source)
		}
		if !json.Valid([]byte(profile.Payload)) {
			t.Errorf("payload must stay jsonb-safe: %q", profile.Payload)
		}
	}
}

func TestEnrichBatchWrapsNonJSONUpstream(t *testing.T) {
	// A payload that stops being JSON must not break the jsonb column: the
	// record is rewritten with an external ref.
	f := enrichmentFixture(t, http.StatusOK, `<html>not json</html>`)
	job := runEnrichBatch(t, f, `{"count":1}`)

	if job.Status != "done" {
		t.Fatalf("batch: want done, got %+v", job)
	}
	profiles, _, _ := f.store.ListEnrichments(context.Background(), listOpts())
	if len(profiles) != 1 || !strings.Contains(profiles[0].Payload, "external_ref") {
		t.Errorf("fallback payload: want external_ref rewrite, got %+v", profiles)
	}
}

func TestEnrichBatchRetriesThenFailsOnUpstreamOutage(t *testing.T) {
	// A 5xx is transient: the batch retries, still fails, and the job
	// records the upstream status.
	f := enrichmentFixture(t, http.StatusInternalServerError, "boom")
	job := runEnrichBatch(t, f, `{"count":1}`)

	if job.Status != "failed" || !strings.Contains(job.Error, "transient upstream status 500") {
		t.Fatalf("5xx batch: want failed with transient cause, got %+v", job)
	}
	if profiles, _, _ := f.store.ListEnrichments(context.Background(), listOpts()); len(profiles) != 0 {
		t.Errorf("failed batch must store nothing: %d", len(profiles))
	}
}

func TestEnrichBatchDoesNotRetryPermanentFailures(t *testing.T) {
	// A 4xx is permanent: the error is not retried and does not count
	// toward the circuit breaker.
	f := enrichmentFixture(t, http.StatusUnauthorized, `{"error":"nope"}`)
	job := runEnrichBatch(t, f, `{"count":1}`)

	if job.Status != "failed" || !strings.Contains(job.Error, "permanent upstream status 401") {
		t.Fatalf("4xx batch: want failed with permanent cause, got %+v", job)
	}
}
