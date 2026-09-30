#!/bin/sh
# Local progressive-delivery walkthrough (ADR 0024): a kind cluster running
# the api as an Argo Rollouts canary gated by the SLO AnalysisTemplate.
#
#   scripts/kind-demo.sh up      # cluster + prometheus + argo-rollouts + app
#   scripts/kind-demo.sh release # push a new revision, watch the canary gate
#   scripts/kind-demo.sh down    # tear it all down
#
# Requirements: docker, kind, kubectl, helm. Skips steps already done.
set -eu
CLUSTER=go-template-demo
APP_NS=go-template
OBS_NS=observability
ROLLOUTS_VERSION=v1.7.2
IMAGE=go-template:kind-demo

step() { printf '\n==> %s\n' "$1"; }

have() { command -v "$1" >/dev/null 2>&1; }
for tool in docker kind kubectl helm; do
  have "$tool" || { echo "missing dependency: $tool"; exit 1; }
done

cluster_exists() { kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; }

up() {
  step "1/6 kind cluster ($CLUSTER)"
  cluster_exists || kind create cluster --name "$CLUSTER" --wait 90s

  step "2/6 building and loading the image"
  docker build --build-arg VERSION=kind-demo -t "$IMAGE" .
  kind load docker-image "$IMAGE" --name "$CLUSTER"

  step "3/6 prometheus (scrapes the app's app_* metrics)"
  kubectl create namespace "$OBS_NS" --dry-run=client -o yaml | kubectl apply -f -
  if ! kubectl -n "$OBS_NS" get svc prometheus-server >/dev/null 2>&1; then
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
    helm repo update >/dev/null
    cat > /tmp/prom-kind-values.yaml <<'EOF'
server:
  global:
    scrape_interval: 10s
# Discover the app's pods and scrape /metrics on the http port (8080).
extraScrapeConfigs: |
  - job_name: go-template
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_name]
        regex: go-template
        action: keep
      - source_labels: [__meta_kubernetes_pod_ip]
        regex: (.*)
        target_label: __address__
        replacement: ${1}:8080
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: pod
EOF
    helm install prometheus prometheus-community/prometheus \
      -n "$OBS_NS" -f /tmp/prom-kind-values.yaml
  fi

  step "4/6 argo-rollouts"
  kubectl create namespace argo-rollouts --dry-run=client -o yaml | kubectl apply -f -
  kubectl -n argo-rollouts get deploy argo-rollouts >/dev/null 2>&1 || \
    kubectl -n argo-rollouts apply -f "https://github.com/argoproj/argo-rollouts/releases/download/$ROLLOUTS_VERSION/install.yaml"
  kubectl -n argo-rollouts rollout status deploy/argo-rollouts --timeout=180s

  step "5/6 installing go-template as a Rollout (canary + analysis)"
  kubectl create namespace "$APP_NS" --dry-run=client -o yaml | kubectl apply -f -
  helm upgrade --install go-template deploy/helm/go-template \
    -n "$APP_NS" \
    --set image.tag=kind-demo \
    --set rollout.enabled=true \
    --set api.hpa.enabled=false \
    --set api.replicaCount=3 \
    --set podDisruptionBudget.enabled=false \
    --set config.databaseURL= \
    --set config.redisAddr= \
    --set rollout.analysis.prometheusAddress="http://prometheus-server.$OBS_NS.svc.cluster.local:80"

  step "6/6 watch it"
  cat <<EOF

The cluster is ready. Watch the canary machinery:

  kubectl -n $APP_NS argo rollouts get rollout go-template-api --watch
  kubectl -n $APP_NS port-forward svc/go-template-api 8080:80

The empty database/redis URLs above run the app on its in-memory store —
this demo is about delivery mechanics, not data.

Next: scripts/kind-demo.sh release
EOF
}

release() {
  step "pushing a new revision (tag change -> new ReplicaSet -> canary)"
  helm upgrade go-template deploy/helm/go-template \
    -n "$APP_NS" \
    --reuse-values \
    --set image.tag=kind-demo \
    --set config.extraEnv.KIND_DEMO_REVISION="$(date +%s)"

  cat <<EOF

A canary rollout started: 20% -> analysis -> 50% -> analysis -> 100%.

  kubectl -n $APP_NS argo rollouts get rollout go-template-api --watch

Each analysis polls Prometheus three times; if availability drops below
99% or p95 exceeds 500ms, the rollout aborts back to stable on its own.
Force that path by pointing the canary at a bad image:

  kubectl -n $APP_NS argo rollouts set image go-template-api \
    api=ghcr.io/your-org/go-template:nope
EOF
}

down() {
  step "deleting cluster"
  cluster_exists && kind delete cluster --name "$CLUSTER"
}

case "${1:-}" in
  up) up ;;
  release) release ;;
  down) down ;;
  *) echo "usage: $0 up|release|down" >&2; exit 1 ;;
esac
