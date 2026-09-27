package httpauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TokenSource supplies Bearer access tokens for outbound service calls: the
// order service account's API key (APIKeyTokenSource), or a StaticToken
// override. Service accounts have no password, so there is no login source.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticToken returns a fixed bearer token (e.g. DUPLI1_ORDER_STOCK_BEARER_TOKEN (or deprecated DUPLI1_INVENTORY_BEARER_TOKEN)).
type StaticToken string

func (s StaticToken) Token(context.Context) (string, error) {
	return string(s), nil
}

func expiryFromJWT(token string, now time.Time) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return now.Add(14 * time.Minute)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return now.Add(14 * time.Minute)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return now.Add(14 * time.Minute)
	}
	return time.Unix(claims.Exp, 0)
}

// doJSON sends req and decodes a 2xx JSON body into target; any other status
// becomes an error carrying the response's "error" field.
func doJSON(client *http.Client, req *http.Request, target any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error == "" {
			errBody.Error = resp.Status
		}
		return fmt.Errorf("%s", errBody.Error)
	}
	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}
