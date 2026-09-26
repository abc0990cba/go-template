## Purpose

User management provides CRUD and pagination scenarios for practicing REST, SQL, and high-volume list behavior.

## ADDED Requirements

### Requirement: List Users With Pagination Modes

The system shall list users with offset, cursor, and keyset pagination modes.

#### Scenario: Switch pagination mode

- **WHEN** a user chooses a pagination mode in the UI
- **THEN** the list endpoint returns users using the selected mode
