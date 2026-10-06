# deploy/gitops/ — Argo CD app-of-apps

- `application.yaml` — root Argo CD application that manages all other apps declaratively (app-of-apps pattern); point it at your overlay repo and let Argo sync from there.

Docs: [../../docs/deployment/gitops.md](../../docs/deployment/gitops.md)
