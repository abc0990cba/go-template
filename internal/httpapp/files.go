package httpapp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/service"
)

// apiUploadFile stores a multipart upload (POST /api/files). Fields: file
// (required), ref_type + ref_id (optional attachment target; "ticket" is
// validated against ticket visibility). The global body limit allows the
// configured upload cap on this route.
func (a *App) apiUploadFile(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	header, err := c.FormFile("file")
	if err != nil {
		var overflow *http.MaxBytesError
		if errors.As(err, &overflow) {
			a.problem(c, http.StatusRequestEntityTooLarge, "request body too large", nil)
			return
		}
		a.problem(c, http.StatusBadRequest, "multipart field 'file' is required", nil)
		return
	}
	if header.Size > a.services.Files.MaxSize() {
		a.problem(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds the %d MiB limit", a.services.Files.MaxSize()>>20), nil)
		return
	}
	opened, err := header.Open()
	if err != nil {
		a.problem(c, http.StatusBadRequest, "cannot read upload", err)
		return
	}
	defer func() { _ = opened.Close() }()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	result, err := a.services.Files.Upload(c.Request.Context(), user.ID, domain,
		header.Filename, contentType, header.Size, opened,
		c.PostForm("ref_type"), c.PostForm("ref_id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "file.uploaded", "file", result.File.ID, nil, gin.H{
		"filename": result.File.Filename, "size": result.File.SizeBytes, "ref": result.File.RefType,
	})
	// Ticket attachments land on the timeline so approvers see them.
	if result.File.RefType == service.FileRefTicket {
		a.services.Tickets.RecordAttachment(c.Request.Context(), user.ID, result.File.RefID, result.File.Filename)
	}
	c.JSON(http.StatusCreated, result.File)
}

// apiGetFile returns one file's metadata.
func (a *App) apiGetFile(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	file, err := a.services.Files.GetForActor(c.Request.Context(), user.ID, domain, c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, file)
}

// apiDownloadFile streams the blob through the API (works in every mode).
func (a *App) apiDownloadFile(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	file, stream, err := a.services.Files.Download(c.Request.Context(), user.ID, domain, c.Param("id"))
	if err != nil {
		a.fail(c, err)
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

// apiFilePresign hands out a direct URL when the store supports it.
func (a *App) apiFilePresign(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	url, ttl, err := a.services.Files.Presign(c.Request.Context(), user.ID, domain, c.Param("id"))
	if err != nil {
		if errors.Is(err, filestore.ErrNotSupported) {
			a.problem(c, http.StatusNotImplemented, "the active file store cannot presign URLs", nil)
			return
		}
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "expires_in": int(ttl.Seconds())})
}

// apiDeleteFile soft-deletes metadata and drops the blob (owner only).
func (a *App) apiDeleteFile(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	if err := a.services.Files.Delete(c.Request.Context(), user.ID, domain, c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "file.deleted", "file", c.Param("id"), nil, nil)
	c.Status(http.StatusNoContent)
}

// apiTicketAttachments lists one ticket's attachments after the ticket
// visibility check (reuse for the detail page).
func (a *App) apiTicketAttachments(c *gin.Context) {
	user := a.currentUser(c)
	domain := a.requestDomain(c)
	files, err := a.services.Files.ListByRef(c.Request.Context(), user.ID, domain, service.FileRefTicket, c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": files})
}
