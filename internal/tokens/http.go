// Token-grant HTTP surface: the OAuth-style endpoints answer on the public
// group under the tight auth rate budget. The deployment must configure
// APP_JWT_SECRET for them to exist functionally (otherwise 501).

package tokens

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/mfa"
)

// HTTP carries the token-grant handlers' dependencies.
type HTTP struct {
	Svc    *Service
	Audit  *audit.Service
	Logger *slog.Logger
	// AuthRateLimit is the assembly's tighter per-IP budget for the grant
	// endpoints.
	AuthRateLimit gin.HandlerFunc
}

// RegisterHTTP mounts the grant endpoints on the public group.
func RegisterHTTP(public *gin.RouterGroup, h HTTP) {
	public.POST("/auth/token", h.AuthRateLimit, h.grant)
	public.POST("/auth/token/revoke", h.AuthRateLimit, h.revoke)
}

// tokenResponse shapes the OAuth-flavored grant response.
func tokenResponse(grant *GrantResult) gin.H {
	return gin.H{
		"access_token":  grant.AccessToken,
		"token_type":    grant.TokenType,
		"expires_in":    grant.ExpiresIn,
		"refresh_token": grant.RefreshToken,
	}
}

// grant is the OAuth-style grant endpoint: password and refresh_token
// grants. MFA-enrolled accounts get the challenge bridge.
func (h *HTTP) grant(c *gin.Context) {
	var req struct {
		GrantType    string `json:"grant_type"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		RefreshToken string `json:"refresh_token"`
		Code         string `json:"code"`
		Challenge    string `json:"challenge"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	ctx := c.Request.Context()
	ua, ip := c.Request.UserAgent(), c.ClientIP()
	switch req.GrantType {
	case "password":
		user, grant, challenge, err := h.Svc.PasswordGrant(ctx, req.Email, req.Password, ua, ip)
		if err != nil {
			h.grantError(c, err)
			return
		}
		if challenge != "" {
			c.JSON(http.StatusUnauthorized, gin.H{"mfa_required": true, "challenge": challenge})
			return
		}
		audit.Record(h.Audit, c, "token.granted", "user", user.ID, nil, gin.H{"grant": "password"})
		c.JSON(http.StatusOK, tokenResponse(grant))
	case "mfa_challenge":
		user, grant, err := h.Svc.CompleteChallengeGrant(ctx, req.Challenge, req.Code, ua, ip)
		if err != nil {
			h.grantError(c, err)
			return
		}
		audit.Record(h.Audit, c, "token.granted", "user", user.ID, nil, gin.H{"grant": "mfa_challenge"})
		c.JSON(http.StatusOK, tokenResponse(grant))
	case "refresh_token":
		_, grant, err := h.Svc.RefreshGrant(ctx, req.RefreshToken, ua, ip)
		if err != nil {
			h.grantError(c, err)
			return
		}
		c.JSON(http.StatusOK, tokenResponse(grant))
	default:
		httpx.Problem(c, http.StatusBadRequest, "unsupported grant_type", nil)
	}
}

// revoke kills a refresh token; unknown tokens answer 204 the same.
func (h *HTTP) revoke(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.Revoke(c.Request.Context(), req.RefreshToken); err != nil {
		if errors.Is(err, ErrAuthDisabled) {
			httpx.Problem(c, http.StatusNotImplemented, "token authentication is not configured", nil)
			return
		}
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTP) grantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrAuthDisabled):
		httpx.Problem(c, http.StatusNotImplemented, "token authentication is not configured", nil)
	case errors.Is(err, ErrTokenReuse):
		httpx.Problem(c, http.StatusUnauthorized, "refresh token reuse detected; the token family was revoked", nil)
	case errors.Is(err, mfa.ErrEnrollmentRequired):
		c.JSON(http.StatusForbidden, gin.H{
			"type": "about:blank", "title": "two-factor enrollment required", "status": http.StatusForbidden,
			"mfa_enrollment_required": true,
		})
	default:
		// Invalid grants stay uniform: no oracle for which part was wrong.
		httpx.Problem(c, http.StatusUnauthorized, "invalid grant", nil)
	}
}
