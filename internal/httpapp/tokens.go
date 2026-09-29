package httpapp

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/service"
)

// tokenResponse shapes the OAuth-flavored grant response.
func tokenResponse(grant *service.GrantResult) gin.H {
	return gin.H{
		"access_token":  grant.AccessToken,
		"token_type":    grant.TokenType,
		"expires_in":    grant.ExpiresIn,
		"refresh_token": grant.RefreshToken,
	}
}

// apiToken is the OAuth-style grant endpoint: password and refresh_token
// grants. MFA-enrolled accounts get the challenge bridge; the deployment
// must configure APP_JWT_SECRET for the endpoints to exist functionally.
func (a *App) apiToken(c *gin.Context) {
	var req struct {
		GrantType    string `json:"grant_type"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		RefreshToken string `json:"refresh_token"`
		Code         string `json:"code"`
		Challenge    string `json:"challenge"`
	}
	if !a.bind(c, &req) {
		return
	}
	ctx := c.Request.Context()
	ua, ip := c.Request.UserAgent(), c.ClientIP()
	switch req.GrantType {
	case "password":
		user, grant, challenge, err := a.services.Tokens.PasswordGrant(ctx, req.Email, req.Password, ua, ip)
		if err != nil {
			a.tokenGrantError(c, err)
			return
		}
		if challenge != "" {
			c.JSON(http.StatusUnauthorized, gin.H{"mfa_required": true, "challenge": challenge})
			return
		}
		a.audit(c, "token.granted", "user", user.ID, nil, gin.H{"grant": "password"})
		c.JSON(http.StatusOK, tokenResponse(grant))
	case "mfa_challenge":
		user, grant, err := a.services.Tokens.CompleteChallengeGrant(ctx, req.Challenge, req.Code, ua, ip)
		if err != nil {
			a.tokenGrantError(c, err)
			return
		}
		a.audit(c, "token.granted", "user", user.ID, nil, gin.H{"grant": "mfa_challenge"})
		c.JSON(http.StatusOK, tokenResponse(grant))
	case "refresh_token":
		user, grant, err := a.services.Tokens.RefreshGrant(ctx, req.RefreshToken, ua, ip)
		if err != nil {
			a.tokenGrantError(c, err)
			return
		}
		_ = user
		c.JSON(http.StatusOK, tokenResponse(grant))
	default:
		a.problem(c, http.StatusBadRequest, "unsupported grant_type", nil)
	}
}

// apiTokenRevoke kills a refresh token; unknown tokens answer 204 the same.
func (a *App) apiTokenRevoke(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !a.bind(c, &req) {
		return
	}
	if err := a.services.Tokens.Revoke(c.Request.Context(), req.RefreshToken); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *App) tokenGrantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrTokenAuthDisabled):
		a.problem(c, http.StatusNotImplemented, "token authentication is not configured", nil)
	case errors.Is(err, service.ErrTokenReuse):
		a.problem(c, http.StatusUnauthorized, "refresh token reuse detected; the token family was revoked", nil)
	case errors.Is(err, service.ErrMFAEnrollmentRequired):
		c.JSON(http.StatusForbidden, gin.H{
			"type": "about:blank", "title": "two-factor enrollment required", "status": http.StatusForbidden,
			"mfa_enrollment_required": true,
		})
	default:
		// Invalid grants stay uniform: no oracle for which part was wrong.
		a.problem(c, http.StatusUnauthorized, "invalid grant", nil)
	}
}
