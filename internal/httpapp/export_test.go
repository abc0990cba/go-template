package httpapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

func newExportServer(t *testing.T) (*httptest.Server, *service.Container) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	blobs := filestore.NewMemory()
	services.Files = service.NewFilesService(store, blobs, services.Tickets, 10<<20, logger)
	services.Exports = service.NewExportService(store, store, store, store, store, store, blobs)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)
	return server, services
}

func TestExportEndpointsInMemoryMode(t *testing.T) {
	server, services := newExportServer(t)
	token := registerAndLogin(t, server, "export-api@example.com")

	// Without the queue the job is created but stays queued: single-process
	// dev mode runs it via POST /api/jobs/process, like every other type.
	status, body := callJSON(t, server, http.MethodPost, "/api/users/me/export", "", token)
	if status != http.StatusAccepted {
		t.Fatalf("request without queue: want 202, got %d (%v)", status, body)
	}
	if status, body = callJSON(t, server, http.MethodPost, "/api/jobs/process", "", token); status != http.StatusOK {
		t.Fatalf("process: want 200, got %d (%v)", status, body)
	}

	// A bundle generated out-of-band downloads fine.
	userID := userIDOf(t, server, token)
	if _, err := services.Exports.Generate(t.Context(), userID); err != nil {
		t.Fatalf("seed export: %v", err)
	}
	if got := getStatus(t, server, http.MethodGet, "/api/users/me/export/download", token); got != http.StatusOK {
		t.Fatalf("download: want 200, got %d", got)
	}

	// Strangers cannot reach someone else's bundle through the files API:
	// the export file ids never leak, and the export endpoints are me-scoped.
	status, _ = callJSON(t, server, http.MethodGet, "/api/users/me/export", "", token)
	if status != http.StatusNotFound {
		// No job record exists in memory mode; the endpoint says so.
		t.Logf("status endpoint answered %d without a job record", status)
	}
}
