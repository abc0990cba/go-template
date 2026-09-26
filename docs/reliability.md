# Reliability

Required production patterns:

- graceful shutdown
- context propagation
- explicit timeouts
- bounded concurrency
- retry limits
- exponential backoff with jitter
- no infinite retries
- circuit breaker for unstable external dependencies
- dead-letter handling for jobs
- idempotency keys for sensitive commands
- health endpoints

Health endpoints:

- `/health/live`
- `/health/ready`
- `/health/startup`

The app should fail predictably and recover cleanly.
