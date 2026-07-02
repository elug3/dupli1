package httpauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// FetchAccessToken logs in and refreshes to obtain a short-lived access token.
func FetchAccessToken(ctx context.Context, authBaseURL, email, password string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(authBaseURL, "/")

	var loginResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := postJSON(ctx, client, base+"/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, &loginResp); err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	if loginResp.RefreshToken == "" {
		return "", fmt.Errorf("login: missing refresh_token")
	}

	var refreshResp struct {
		Token string `json:"token"`
	}
	if err := postJSON(ctx, client, base+"/api/v1/auth/refresh", map[string]string{
		"refresh_token": loginResp.RefreshToken,
	}, &refreshResp); err != nil {
		return "", fmt.Errorf("refresh: %w", err)
	}
	if refreshResp.Token == "" {
		return "", fmt.Errorf("refresh: missing token")
	}
	return refreshResp.Token, nil
}

func postJSON(ctx context.Context, client *http.Client, url string, body any, target any) error {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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
