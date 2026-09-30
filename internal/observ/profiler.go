package observ

import (
	"github.com/grafana/pyroscope-go"
)

// StartProfiler begins continuous profiling against a Pyroscope server and
// returns the stop function. An empty URL keeps profiling off — the default,
// so local runs and unit tests never open background connections.
//
// Profiling answers the question metrics cannot: WHICH code is burning CPU
// or allocating. CPU, in-use, and allocation profiles are pushed every
// cycle; in Grafana they appear as flame graphs under the Pyroscope
// datasource (and next to metrics via the tracing/metrics views).
func StartProfiler(url string, build Build) func() {
	if url == "" {
		return func() {}
	}
	controller, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "go-template",
		ServerAddress:   url,
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"version": build.Version},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
		},
	})
	if err != nil {
		// Profiling must never take the app down: degrade to no-op.
		return func() {}
	}
	return func() { _ = controller.Stop() }
}
