package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/gin-gonic/gin"
)

// apiKeyResponse is a key as the management API shows it. The plaintext is
// only ever present in the create response.
type apiKeyResponse struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	APIKey      string     `json:"api_key,omitempty"`
	Prefix      string     `json:"prefix"`
	Permissions []string   `json:"permissions"`
	Source      string     `json:"source"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

func toAPIKeyResponse(k *domain.APIKey) apiKeyResponse {
	perms := k.Permissions
	if perms == nil {
		perms = []string{}
	}
	return apiKeyResponse{
		ID: k.ID, UserID: k.UserID, Name: k.Name, Prefix: k.Prefix, Permissions: perms,
		Source: k.Source, CreatedAt: k.CreatedAt, CreatedBy: k.CreatedBy,
		ExpiresAt: k.ExpiresAt, LastUsedAt: k.LastUsedAt, RevokedAt: k.RevokedAt,
	}
}

// ExchangeAPIKey handles POST /api/v1/auth/token with
// `Authorization: ApiKey <key>` and answers an access token. Every failure is
// the same 401 so a caller cannot probe which check tripped; the reason goes
// to the log. The gateway serves this only on its internal listener.
func (h *Handler) ExchangeAPIKey(c *gin.Context) {
	ip := c.ClientIP()
	plaintext, ok := apiKeyFromHeader(c.GetHeader("Authorization"))
	if !ok {
		h.logger.Warn().Str("event", "api_key_rejected").Str("reason", "missing").Str("ip", ip).
			Msg("api key exchange: no ApiKey authorization")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_key"})
		return
	}
	res, err := h.svc.ExchangeAPIKey(c.Request.Context(), plaintext)
	if err != nil {
		if errors.Is(err, autherrors.ErrInvalidAPIKey) {
			ev := h.logger.Warn().Str("event", "api_key_rejected").Str("reason", string(res.Reason)).Str("ip", ip)
			if res.Key != nil {
				ev = ev.Str("key_id", res.Key.ID).Str("prefix", res.Key.Prefix).Str("user_id", res.Key.UserID)
			}
			ev.Msg("api key exchange refused")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_key"})
			return
		}
		// A store outage is not a bad key: answer 503 so the caller keeps its
		// key and retries, as /refresh does.
		h.logger.Error().Err(err).Str("event", "api_key_exchange_error").Str("ip", ip).Msg("api key exchange failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "token exchange unavailable"})
		return
	}
	h.logger.Info().Str("event", "api_key_exchanged").Str("key_id", res.Key.ID).Str("prefix", res.Key.Prefix).
		Str("user_id", res.Key.UserID).Str("ip", ip).Msg("api key exchanged")
	c.JSON(http.StatusOK, gin.H{
		"token":      res.AccessToken,
		"token_type": "Bearer",
		"expires_in": int(res.ExpiresIn.Seconds()),
	})
}

// apiKeyFromHeader reads `ApiKey <key>`; the scheme is case-insensitive
// (RFC 7235), the key is not.
func apiKeyFromHeader(v string) (string, bool) {
	const scheme = "apikey "
	if len(v) <= len(scheme) || !strings.EqualFold(v[:len(scheme)], scheme) {
		return "", false
	}
	key := strings.TrimSpace(v[len(scheme):])
	return key, key != ""
}

// manageableServiceAccount loads service account :id and checks the caller
// may manage it, writing the error response when not.
func (h *Handler) manageableServiceAccount(c *gin.Context, userID string) (*domain.User, bool) {
	target, err := h.svc.ServiceAccount(c.Request.Context(), userID)
	switch {
	case errors.Is(err, autherrors.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return nil, false
	case errors.Is(err, autherrors.ErrNotServiceAccount):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_account_type", "message": err.Error()})
		return nil, false
	case err != nil:
		h.respondInternalError(c, "api_key_owner_lookup_error", err)
		return nil, false
	}
	if !domain.CanManageUser(callerFromContext(c), target) {
		c.JSON(http.StatusForbidden, gin.H{"error": autherrors.ErrManagementForbidden.Error()})
		return nil, false
	}
	return target, true
}

// ListAPIKeys handles GET /users/:id/api-keys. Requires user.apikey.read.
func (h *Handler) ListAPIKeys(c *gin.Context) {
	target, ok := h.manageableServiceAccount(c, c.Param("id"))
	if !ok {
		return
	}
	keys, err := h.svc.ListAPIKeys(c.Request.Context(), target.ID)
	if err != nil {
		h.respondInternalError(c, "list_api_keys_error", err)
		return
	}
	out := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		out[i] = toAPIKeyResponse(k)
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": out})
}

// CreateAPIKey handles POST /users/:id/api-keys. Requires user.apikey.manage.
// The response is the only place the plaintext key ever appears.
func (h *Handler) CreateAPIKey(c *gin.Context) {
	var body struct {
		Name          string   `json:"name"`
		Permissions   []string `json:"permissions"`
		ExpiresInDays int      `json:"expires_in_days"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "create api key: parse request: " + err.Error()})
		return
	}
	if body.ExpiresInDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expires_in_days must be positive (omit it for no expiry)"})
		return
	}
	target, ok := h.manageableServiceAccount(c, c.Param("id"))
	if !ok {
		return
	}
	caller := callerFromContext(c)
	ttl := time.Duration(body.ExpiresInDays) * 24 * time.Hour
	key, plaintext, err := h.svc.CreateAPIKey(c.Request.Context(), target.ID, body.Name, body.Permissions, ttl, caller.ID)
	if err != nil {
		switch {
		case errors.Is(err, autherrors.ErrInvalidAPIKeyRequest),
			errors.Is(err, autherrors.ErrInvalidPermission),
			errors.Is(err, autherrors.ErrScopeExceedsAccount):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			h.respondInternalError(c, "create_api_key_error", err)
		}
		return
	}
	h.logger.Info().Str("event", "api_key_created").Str("key_id", key.ID).Str("prefix", key.Prefix).
		Str("user_id", key.UserID).Str("created_by", caller.ID).Int("scope_count", len(key.Permissions)).
		Msg("api key created")
	resp := toAPIKeyResponse(key)
	resp.APIKey = plaintext
	c.JSON(http.StatusCreated, resp)
}

// RevokeAPIKey handles DELETE /api-keys/:keyId. Requires user.apikey.manage.
func (h *Handler) RevokeAPIKey(c *gin.Context) {
	key, err := h.svc.FindAPIKey(c.Request.Context(), c.Param("keyId"))
	if err != nil {
		if errors.Is(err, autherrors.ErrAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		} else {
			h.respondInternalError(c, "revoke_api_key_lookup_error", err)
		}
		return
	}
	if _, ok := h.manageableServiceAccount(c, key.UserID); !ok {
		return
	}
	if _, err := h.svc.RevokeAPIKey(c.Request.Context(), key.ID); err != nil {
		if errors.Is(err, autherrors.ErrEnvManagedKey) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "env_managed_key",
				"message": "this key is seeded from the service's *_SERVICE_API_KEY environment variable; change or unset that and restart auth to rotate or revoke it",
			})
		} else {
			h.respondInternalError(c, "revoke_api_key_error", err)
		}
		return
	}
	h.logger.Info().Str("event", "api_key_revoked").Str("key_id", key.ID).Str("prefix", key.Prefix).
		Str("revoked_by", callerFromContext(c).ID).Msg("api key revoked")
	c.Status(http.StatusNoContent)
}
