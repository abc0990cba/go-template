# Nomad alternative (ADR 0024): the same api + worker pair as a Nomad job.
# Nomad's `update` block gives canary + auto-revert natively — no extra
# controller: one canary instance runs beside stable, health checks must
# pass, and a failed deployment reverts automatically.
#
# Local walkthrough (single dev agent on your machine, Docker driver):
#   make nomad-up      # nomad agent -dev in the background
#   make nomad-run     # submit this job
#   open http://localhost:4646   # watch the deployment + canary placement
#   make nomad-stop    # stop the agent
#
# Production notes: replace the service blocks' providers, add consul/vault
# integrations, and point env blocks at your secret store instead of inline
# defaults.
job "go-template" {
  type = "service"

  group "api" {
    count = 2

    network {
      port "http" {
        static = 8080
        to     = 8080
      }
    }

    service {
      name     = "go-template-api"
      port     = "http"
      provider = "nomad"

      check {
        type     = "http"
        path     = "/health/ready"
        interval = "10s"
        timeout  = "2s"
      }
    }

    update {
      max_parallel     = 1
      min_healthy_time = "15s"
      healthy_deadline = "3m"
      # The canary story: one canary alloc runs beside stable; promotion
      # waits for health; a failed deployment reverts without a human.
      canary           = 1
      auto_promote     = true
      auto_revert      = true
    }

    task "api" {
      driver = "docker"

      config {
        image = "ghcr.io/your-org/go-template:latest"
        ports = ["http"]
        args  = ["/app/api"]
      }

      env {
        APP_HTTP_ADDR      = ":8080"
        APP_DATABASE_URL   = "postgres://app:app@host.docker.internal:55432/app?sslmode=disable"
        APP_REDIS_ADDR     = "host.docker.internal:6379"
        APP_LOG_FORMAT     = "json"
        APP_COOKIE_SECURE  = "0"
        APP_OTLP_ENDPOINT  = "host.docker.internal:4318"
        APP_DEMO_SEED      = "1"
        APP_DEMO_EMAIL     = "demo@go-template.local"
        APP_DEMO_PASSWORD  = "demo-password-123"
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }
  }

  group "worker" {
    count = 1

    task "worker" {
      driver = "docker"

      config {
        image = "ghcr.io/your-org/go-template:latest"
        args  = ["/app/worker"]
      }

      env {
        APP_DATABASE_URL   = "postgres://app:app@host.docker.internal:55432/app?sslmode=disable"
        APP_REDIS_ADDR     = "host.docker.internal:6379"
        APP_LOG_FORMAT     = "json"
        APP_OTLP_ENDPOINT  = "host.docker.internal:4318"
        APP_METRICS_ADDR   = ":9090"
      }

      resources {
        cpu    = 200
        memory = 128
      }
    }
  }
}
