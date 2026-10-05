//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/repository"
)

func TestQueueEndToEnd(t *testing.T) {
	ctx := context.Background()
	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("start redis container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("redis host: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		t.Fatalf("redis port: %v", err)
	}
	addr := host + ":" + port.Port()

	store := repository.NewMemoryStore()
	jobQueue := queue.New(addr)
	t.Cleanup(func() { _ = jobQueue.Close() })
	services := bootstrap.NewContainer(bootstrap.Deps{
		Auth: store, Users: store, Jobs: store, Reports: store,
		Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, APIKeys: store, Webhooks: store, Mailer: nil, Queue: jobQueue,
	}, testLogger(), testConfig(t))

	// The worker side: the same mux the worker binary runs.
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: addr}, asynq.Config{Concurrency: 2})
	mux := queue.NewMux(services.Jobs, testLogger(), services.Webhooks, services.Email)
	go func() { _ = server.Run(mux) }()
	t.Cleanup(server.Shutdown)

	t.Run("report job runs through redis", func(t *testing.T) {
		before, _, err := services.Reports.List(ctx, kernel.ListOptions{Mode: "offset", Limit: 100})
		if err != nil {
			t.Fatalf("list reports: %v", err)
		}
		if _, err := services.Jobs.Enqueue(ctx, jobs.TypeReportGenerate, "{}", ""); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForJob(t, ctx, services, "done", 15*time.Second)

		after, _, err := services.Reports.List(ctx, kernel.ListOptions{Mode: "offset", Limit: 100})
		if err != nil {
			t.Fatalf("list reports: %v", err)
		}
		if len(after) != len(before)+1 {
			t.Errorf("reports: want %d, got %d", len(before)+1, len(after))
		}
	})

	t.Run("task ids dedupe", func(t *testing.T) {
		now := time.Now()
		job := jobs.Job{ID: "dedup-1", Type: jobs.TypeReportGenerate, Status: "queued", CreatedAt: now, UpdatedAt: now}
		if err := jobQueue.Enqueue(ctx, job); err != nil {
			t.Fatalf("first enqueue: %v", err)
		}
		if err := jobQueue.Enqueue(ctx, job); !errors.Is(err, asynq.ErrTaskIDConflict) {
			t.Errorf("duplicate enqueue: want ErrTaskIDConflict, got %v", err)
		}
	})

	t.Run("unknown task type is rejected", func(t *testing.T) {
		if _, err := services.Jobs.Enqueue(ctx, "bogus.type", "{}", ""); err == nil {
			t.Fatal("bogus type accepted")
		}
	})
}

func waitForJob(t *testing.T, ctx context.Context, services *bootstrap.Container, wantStatus string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		jobs, _, err := services.Jobs.List(ctx, kernel.ListOptions{Mode: "offset", Limit: 100})
		if err != nil {
			t.Fatalf("list jobs: %v", err)
		}
		if len(jobs) > 0 && jobs[0].Status == wantStatus {
			return
		}
		if len(jobs) > 0 && jobs[0].Status == "failed" {
			t.Fatalf("job failed: %s", jobs[0].Error)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job did not reach %s within %s", wantStatus, timeout)
}
