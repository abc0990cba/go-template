// Production job spec for the go-template application stack.
//
// This file is the SINGLE source of truth for app deploys and is consumed by
// both deploy paths unchanged:
//   - `make vps-deploy` / `make vps-app-deploy TAG=…` (Ansible passes -var)
//   - the Woodpecker pipeline on the VPS (.woodpecker/build-deploy.yml)
//
// Design notes:
//   - Secrets never appear here. Tasks mount the host "secrets" volume
//     read-only at /secrets and read APP_*_FILE variables (the app's native
//     docker-secrets pattern).
//   - host networking: every service binds the node's loopback (Postgres,
//     Redis, worker metrics); only the api listens on :8080 and only Caddy
//     exposes 80/443 publicly (UFW enforces this).
//   - The update block gives Nomad-native canary + auto-revert: a new api
//     allocation must pass /health/ready before promotion; a failing
//     deployment reverts to the previous version without human action.

variable "app_image" {
  type        = string
  description = "Container image repository (no tag)."
  default     = "127.0.0.1:5000/go-template"
}

variable "image_tag" {
  type        = string
  description = "Image tag to deploy (immutable; produced by CI or the bootstrap build)."
  default     = "v0.1.0"
}

variable "app_domain" {
  type        = string
  description = "Public hostname served by Caddy."
  default     = "localhost"
}

variable "api_replicas" {
  type        = number
  description = "api allocation count (multi-node scaling is a Phase-2 change)."
  default     = 1
}

variable "report_cron" {
  type    = string
  default = "0 3 * * *"
}

variable "smtp_host" {
  type    = string
  default = ""
}

variable "smtp_port" {
  type    = string
  default = "587"
}

variable "smtp_username" {
  type    = string
  default = ""
}

variable "smtp_from" {
  type    = string
  default = ""
}

locals {
  base_url = "https://${var.app_domain}"
}

job "go-template" {
  type = "service"

  # ---------------------------------------------------------------- api -----
  group "api" {
    count = var.api_replicas

    volume "secrets" {
      type      = "host"
      source    = "secrets"
      read_only = true
    }

    update {
      max_parallel     = 1
      min_healthy_time = "15s"
      healthy_deadline = "5m"
      canary           = 1
      auto_promote     = true
      auto_revert      = true
      prune            = true
    }

    restart {
      attempts = 3
      interval = "10m"
      delay    = "15s"
      mode     = "delay"
    }

    service {
      name     = "go-template-api"
      port     = "8080"
      provider = "nomad"

      check {
        type     = "http"
        path     = "/health/ready"
        interval = "10s"
        timeout  = "3s"
      }
    }

    task "api" {
      driver = "docker"
      # Graceful drain window: the app's APP_SHUTDOWN_TIMEOUT is 10s.
      kill_timeout = "30s"

      config {
        image        = "${var.app_image}:${var.image_tag}"
        args         = ["/app/api"]
        network_mode = "host"
      }

      volume_mount {
        volume      = "secrets"
        destination = "/secrets"
        read_only   = true
      }

      env {
        APP_HTTP_ADDR          = ":8080"
        APP_BASE_URL           = "${local.base_url}"
        APP_DATABASE_URL_FILE  = "/secrets/database_url"
        APP_REDIS_ADDR         = "127.0.0.1:6379"
        APP_JWT_SECRET_FILE    = "/secrets/jwt_secret"
        APP_LOG_LEVEL          = "info"
        APP_LOG_FORMAT         = "json"
        APP_COOKIE_SECURE      = "1"
        APP_REPORT_CRON        = "${var.report_cron}"
        APP_SMTP_HOST          = "${var.smtp_host}"
        APP_SMTP_PORT          = "${var.smtp_port}"
        APP_SMTP_USERNAME      = "${var.smtp_username}"
        APP_SMTP_FROM          = "${var.smtp_from}"
        APP_SMTP_PASSWORD_FILE = "/secrets/smtp_password"
      }

      resources {
        cpu    = 500
        memory = 384
      }
    }
  }

  # ------------------------------------------------------------- worker -----
  group "worker" {
    count = 1

    volume "secrets" {
      type      = "host"
      source    = "secrets"
      read_only = true
    }

    update {
      max_parallel     = 1
      min_healthy_time = "10s"
      healthy_deadline = "5m"
      auto_revert      = true
      prune            = true
    }

    restart {
      attempts = 3
      interval = "10m"
      delay    = "15s"
      mode     = "delay"
    }

    service {
      name     = "go-template-worker"
      port     = "9090"
      provider = "nomad"

      check {
        type     = "http"
        path     = "/metrics"
        interval = "15s"
        timeout  = "3s"
      }
    }

    task "worker" {
      driver = "docker"
      kill_timeout = "30s"

      config {
        image        = "${var.app_image}:${var.image_tag}"
        args         = ["/app/worker"]
        network_mode = "host"
      }

      volume_mount {
        volume      = "secrets"
        destination = "/secrets"
        read_only   = true
      }

      env {
        APP_DATABASE_URL_FILE = "/secrets/database_url"
        APP_REDIS_ADDR        = "127.0.0.1:6379"
        APP_METRICS_ADDR      = "127.0.0.1:9090"
        APP_LOG_LEVEL         = "info"
        APP_LOG_FORMAT        = "json"
        APP_REPORT_CRON       = "${var.report_cron}"
      }

      resources {
        cpu    = 250
        memory = 256
      }
    }
  }
}
