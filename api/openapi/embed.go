// Package openapi embeds the API contract into the binary: the distroless
// image ships no YAML files, so /openapi.yaml and the runtime request
// validator both read from here. The embedded copy is compiled from the same
// file the contract tests lint, so it can never drift from what CI checked.
package openapi

import _ "embed"

// Spec is the raw OpenAPI 3 contract (api/openapi/openapi.yaml).
//
//go:embed openapi.yaml
var Spec []byte
