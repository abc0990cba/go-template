## Purpose

Reliability behavior prevents resource leaks, runaway retries, and uncontrolled failure modes.

## ADDED Requirements

### Requirement: Bounded Retries

The system shall retry transient operations only up to a configured limit.

#### Scenario: External API keeps failing

- **WHEN** all retry attempts fail
- **THEN** the operation returns a controlled error instead of retrying forever
