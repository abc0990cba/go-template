// Files HTTP surface: uploads are self-service (owner-scoped metadata);
// reads are gated by attachment visibility (the ticket canView rules) in
// the service, with the files:read catalog guard layered on top. The
// attachments list rides the ticket detail routes.

package files

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the file handlers' dependencies.
type HTTP struct {
	Svc     *Service
	Tickets TicketView
	Audit   *audit.Service
	Logger  *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the file routes on the authorized group.
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.POST("/files", h.upload)
	// Resumable uploads: sessions own their state in the file store; the
	// metadata row appears only at completion.
	authorized.GET("/files/multipart", h.multipartStatus)
	authorized.POST("/files/multipart", h.initiateMultipart)
	authorized.POST("/files/multipart/parts", h.putMultipartPart)
	authorized.POST("/files/multipart/complete", h.completeMultipart)
	authorized.POST("/files/multipart/abort", h.abortMultipart)
	authorized.GET("/files/:id", h.get)
	authorized.GET("/files/:id/content", h.download)
	authorized.GET("/files/:id/url", h.presign)
	authorized.DELETE("/files/:id", h.remove)
	authorized.GET("/tickets/:id/attachments", h.RequirePermission("tickets", "read"), h.ticketAttachments)
}

// upload stores a multipart upload (POST /files). Fields: file (required),
// ref_type + ref_id (optional attachment target; "ticket" is validated
// against ticket visibility). The body-limit middleware allows the
// configured upload cap on this route.
func (h *HTTP) upload(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	header, err := c.FormFile("file")
	if err != nil {
		var overflow *http.MaxBytesError
		if errors.As(err, &overflow) {
			httpx.Problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
			return
		}
		httpx.Problem(c, http.StatusBadRequest, "multipart field 'file' is required", nil)
		return
	}
	if header.Size > h.Svc.MaxSize() {
		httpx.Problem(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds the %d MiB limit", h.Svc.MaxSize()>>20), nil)
		return
	}
	opened, err := header.Open()
	if err != nil {
		httpx.Problem(c, http.StatusBadRequest, "cannot read upload", err)
		return
	}
	defer func() { _ = opened.Close() }()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	result, err := h.Svc.Upload(c.Request.Context(), user.ID, domain,
		header.Filename, contentType, header.Size, opened,
		c.PostForm("ref_type"), c.PostForm("ref_id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "file.uploaded", "file", result.File.ID, nil, gin.H{
		"filename": result.File.Filename, "size": result.File.SizeBytes, "ref": result.File.RefType,
	})
	// Ticket attachments land on the timeline so approvers see them.
	if result.File.RefType == RefTicket {
		h.Tickets.RecordAttachment(c.Request.Context(), user.ID, result.File.RefID, result.File.Filename)
	}
	c.JSON(http.StatusCreated, result.File)
}

// get returns one file's metadata.
func (h *HTTP) get(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	file, err := h.Svc.GetForActor(c.Request.Context(), user.ID, domain, c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, file)
}

// download streams the blob through the API (works in every mode).
func (h *HTTP) download(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	file, stream, err := h.Svc.Download(c.Request.Context(), user.ID, domain, c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	defer func() { _ = stream.Close() }()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Filename))
	contentType := file.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.DataFromReader(http.StatusOK, file.SizeBytes, contentType, stream, nil)
}

// presign hands out a direct URL when the store supports it.
func (h *HTTP) presign(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	url, ttl, err := h.Svc.Presign(c.Request.Context(), user.ID, domain, c.Param("id"))
	if err != nil {
		if errors.Is(err, filestore.ErrNotSupported) {
			httpx.Problem(c, http.StatusNotImplemented, "the active file store cannot presign URLs", nil)
			return
		}
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "expires_in": int(ttl.Seconds())})
}

// remove soft-deletes metadata and drops the blob (owner only).
func (h *HTTP) remove(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	if err := h.Svc.Delete(c.Request.Context(), user.ID, domain, c.Param("id")); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "file.deleted", "file", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

// ticketAttachments lists one ticket's attachments after the ticket
// visibility check (reuse for the detail page).
func (h *HTTP) ticketAttachments(c *gin.Context) {
	user := users.CurrentUser(c)
	domain := httpx.Domain(c)
	files, err := h.Svc.ListByRef(c.Request.Context(), user.ID, domain, RefTicket, c.Param("id"))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": files})
}
