package handler

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	defaultGuestCookieName   = "dupli1_guest"
	defaultGuestCookieMaxAge = 365 * 24 * 60 * 60 // 1 year
)

// GuestCookieConfig controls the anonymous guest identity cookie.
type GuestCookieConfig struct {
	Name     string
	Secure   bool
	HTTPOnly bool
	MaxAge   int
	Enabled  bool
}

func defaultGuestCookieConfig() GuestCookieConfig {
	return GuestCookieConfig{
		Name:     defaultGuestCookieName,
		Secure:   false, // local Compose is HTTP; set GUEST_COOKIE_SECURE=true in prod
		HTTPOnly: true,
		MaxAge:   defaultGuestCookieMaxAge,
		Enabled:  true,
	}
}

// GuestCookieConfigFromEnv builds cookie settings from environment variables.
func GuestCookieConfigFromEnv() GuestCookieConfig {
	cfg := defaultGuestCookieConfig()
	if v := os.Getenv("GUEST_COOKIE_NAME"); v != "" {
		cfg.Name = v
	}
	if v := os.Getenv("GUEST_COOKIE_SECURE"); v != "" {
		cfg.Secure = v == "1" || v == "true" || v == "TRUE"
	}
	if v := os.Getenv("GUEST_COOKIE_HTTP_ONLY"); v != "" {
		cfg.HTTPOnly = v == "1" || v == "true" || v == "TRUE"
	}
	if v := os.Getenv("GUEST_COOKIE_MAX_AGE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxAge = n
		}
	}
	if v := os.Getenv("PRODUCT_VIEWS_ENABLED"); v != "" {
		cfg.Enabled = v == "1" || v == "true" || v == "TRUE"
	}
	return cfg
}

// ensureGuestID returns the guest id from the cookie, minting one when absent.
// When minted is true, the caller should Set-Cookie on the response.
func (h *Handler) ensureGuestID(r *http.Request) (guestID string, minted bool) {
	cfg := h.guestCookie
	if cfg.Name == "" {
		cfg = defaultGuestCookieConfig()
	}
	if c, err := r.Cookie(cfg.Name); err == nil && c.Value != "" {
		return c.Value, false
	}
	return ulid.Make().String(), true
}

func (h *Handler) setGuestCookie(w http.ResponseWriter, guestID string) {
	cfg := h.guestCookie
	if cfg.Name == "" {
		cfg = defaultGuestCookieConfig()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    guestID,
		Path:     "/",
		MaxAge:   cfg.MaxAge,
		HttpOnly: cfg.HTTPOnly,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().UTC().Add(time.Duration(cfg.MaxAge) * time.Second),
	})
}
