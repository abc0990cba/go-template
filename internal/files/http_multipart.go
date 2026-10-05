package files

// The resumable-upload HTTP surface: initiate → (presigned part PUTs, or
// streamed parts in dev) → complete / abort. The metadata row is created
// only at completion, so an abandoned session leaves no metadata to clean.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/users"
)

type initiateRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Parts       int    `json:"parts"`
	RefType     string `json:"ref_type"`
	RefID       string `json:"ref_id"`
}

// initiateMultipart opens a resumable session (POST /files/multipart).
func (h *HTTP) initiateMultipart(c *gin.Context) {
	var req initiateRequest
	if !httpx.Bind(c, &req) {
		return
	}
	user := users.CurrentUser(c)
	session, err := h.Svc.InitiateMultipart(c.Request.Context(), user.ID, httpx.Domain(c),
		req.Filename, req.ContentType, req.Parts, req.RefType, req.RefID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusCreated, session)
}

type completeRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Key         string `json:"key"`
	UploadID    string `json:"upload_id"`
	RefType     string `json:"ref_type"`
	RefID       string `json:"ref_id"`
	Parts       []struct {
		PartNumber int32  `json:"part_number"`
		ETag       string `json:"etag"`
	} `json:"parts"`
}

// completeMultipart assembles the parts and creates the metadata row
// (POST /files/multipart/complete).
func (h *HTTP) completeMultipart(c *gin.Context) {
	var req completeRequest
	if !httpx.Bind(c, &req) {
		return
	}
	if req.Key == "" || req.UploadID == "" {
		httpx.Problem(c, http.StatusBadRequest, "key and upload_id are required", nil)
		return
	}
	parts := make([]filestore.CompletedPart, 0, len(req.Parts))
	for _, part := range req.Parts {
		parts = append(parts, filestore.CompletedPart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	user := users.CurrentUser(c)
	file, err := h.Svc.CompleteMultipart(c.Request.Context(), user.ID, httpx.Domain(c),
		req.Filename, req.ContentType, req.Key, req.UploadID, parts, req.RefType, req.RefID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "file.uploaded_multipart", "file", file.ID, nil, gin.H{
		"filename": file.Filename, "size": file.SizeBytes, "parts": len(parts), "ref": req.RefType,
	})
	c.JSON(http.StatusCreated, file)
}

// putMultipartPart streams one part through the API (dev mode, where the
// in-process store cannot hand out presigned part URLs).
func (h *HTTP) putMultipartPart(c *gin.Context) {
	uploadID := c.Query("upload_id")
	part := httpx.IntQuery(c, "part_number", 0)
	if uploadID == "" || part < 1 {
		httpx.Problem(c, http.StatusBadRequest, "upload_id and part_number are required", nil)
		return
	}
	// #nosec G115 -- part is validated >= 1 and bounded by the session's
	// part count in the store.
	etag, err := h.Svc.PutMultipartPart(c.Request.Context(), uploadID, int32(part), c.Request.Body)
	if err != nil {
		if errors.Is(err, filestore.ErrNotFound) {
			httpx.Problem(c, http.StatusNotFound, "unknown upload session", err)
			return
		}
		httpx.Fail(c, h.Logger, err)
		return
	}
	// The completion call needs this per part; S3-mode clients read the
	// same header from their presigned PUT response.
	if etag != "" {
		c.Header("ETag", etag)
	}
	c.Status(http.StatusNoContent)
}

type abortRequest struct {
	Key      string `json:"key"`
	UploadID string `json:"upload_id"`
}

// abortMultipart discards a session (POST /files/multipart/abort); the
// upload id is the capability, so no extra permission is needed.
func (h *HTTP) abortMultipart(c *gin.Context) {
	var req abortRequest
	if !httpx.Bind(c, &req) {
		return
	}
	if req.Key == "" || req.UploadID == "" {
		httpx.Problem(c, http.StatusBadRequest, "key and upload_id are required", nil)
		return
	}
	if err := h.Svc.AbortMultipart(c.Request.Context(), req.Key, req.UploadID); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "file.multipart_aborted", "file", req.Key, nil, nil)
	c.Status(http.StatusNoContent)
}

// multipartStatus advertises the capability and its dev-mode shape
// (GET /files/multipart).
func (h *HTTP) multipartStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":   h.Svc.MultipartsEnabled(),
		"streaming": h.Svc.MultipartsEnabled() && !h.Svc.PresignsEnabled(),
		"max_size":  h.Svc.MaxSize(),
	})
}
