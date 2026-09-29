## Purpose

The files capability gives the template object storage: an S3-compatible blob store with an in-process dev fallback, metadata with per-attachment permission semantics, and ticket attachments integrated with the approval timeline.

## ADDED Requirements

### Requirement: Pluggable Blob Storage

Uploads shall be stored through the configured adapter: S3/MinIO when `APP_S3_BUCKET` is configured (with a startup reachability probe and graceful fallback to the in-process store on failure), the in-process store otherwise. Direct presigned URLs shall be available only in S3 mode; other modes answer 501 on the presign endpoint.

#### Scenario: Bucket unreachable degrades

- **WHEN** the app starts with an S3 bucket that cannot be reached
- **THEN** startup succeeds with a warning and uploads use the in-process store

#### Scenario: Presign mode split

- **WHEN** a client requests a presigned URL for a stored file
- **THEN** S3 mode returns a time-limited URL and the in-process store answers 501

### Requirement: Guarded Uploads And Downloads

Uploads shall be capped by the configured per-file limit, checksummed (sha256), and — when attached to a ticket — accepted only if the uploader can see that ticket in the active domain. Downloads and the attachments list shall re-apply the same visibility rules; deletes shall be restricted to the owner. The upload route's body cap shall equal the configured file limit plus multipart framing.

#### Scenario: Attach to an invisible ticket

- **WHEN** a caller who cannot see a ticket uploads with that ticket as the reference
- **THEN** the upload is refused with not-found semantics and no blob is kept

#### Scenario: Roundtrip

- **WHEN** a permitted caller uploads and then downloads a file
- **THEN** the bytes are identical, the filename rides the Content-Disposition, and the metadata carries the matching checksum

### Requirement: Ticket Attachment Timeline

A successful ticket attachment shall append an `attachment_added` entry to the ticket's timeline and notify the ticket's watchers; notification failure shall never fail the upload.

#### Scenario: Approvers see the attachment

- **WHEN** an attachment is uploaded to a submitted ticket
- **THEN** the timeline records the entry and watchers receive a notification
