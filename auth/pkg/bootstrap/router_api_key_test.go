package bootstrap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/handler"
	jwtgen "github.com/elug3/dupli1/auth/pkg/infra/jwt"
	"github.com/elug3/dupli1/auth/pkg/infra/memory"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/auth/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

type apiKeyRouterFixture struct {
	t      *testing.T
	r      *gin.Engine
	svc    *service.Service
	repo   *rbacFakeRepo
	gen    ports.TokenGenerator
	tokens map[string]string // bearer tokens by role
	secret string
}

func newAPIKeyRouterFixture(t *testing.T) *apiKeyRouterFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &apiKeyRouterFixture{t: t, repo: newRBACFakeRepo(), tokens: map[string]string{}, secret: "api-key-test-secret"}
	f.gen = jwtgen.NewTokenGeneratorWithType(f.secret, 900, "access")
	f.svc = service.NewService(f.repo, f.gen, service.WithAPIKeyRepo(memory.NewAPIKeyRepository()),
		service.WithAccessTokenTTL(15*time.Minute))
	f.r = newRouter(handler.NewHandler(f.svc, zerolog.Nop()), false, nil, nil, nil, settings.NewResponse("auth"))

	order := f.user("svc-order", "order@example.com", domain.AccountTypeService,
		permissions.OrderShip, permissions.PromotionRedeem)
	order.ServiceName = "dupli1-order"
	f.user("cust", "cust@example.com", domain.AccountTypeCustomer)
	for role, u := range map[string]*domain.User{
		"owner": f.user("owner", "owner@example.com", domain.AccountTypeManager, permissions.All),
		// user_admin carries user.apikey.*, but an admin still may not manage
		// service accounts — only the owner may.
		"admin":   f.user("admin", "admin@example.com", domain.AccountTypeManager, append(mustBundle(t, permissions.BundleUserAdmin), permissions.AdminAll)...),
		"manager": f.user("mgr", "mgr@example.com", domain.AccountTypeManager, permissions.OrderShip),
	} {
		tok, err := f.gen.Generate(t.Context(), u.ID, u.Permissions, ports.Identity{Email: u.Email})
		if err != nil {
			t.Fatal(err)
		}
		f.tokens[role] = tok
	}
	return f
}

func mustBundle(t *testing.T, name string) []string {
	p, err := permissions.ExpandBundle(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *apiKeyRouterFixture) user(id, email, accountType string, perms ...string) *domain.User {
	u, err := domain.NewUser(id, email, "password12", accountType, perms...)
	if err != nil {
		f.t.Fatal(err)
	}
	_ = f.repo.Save(f.t.Context(), u)
	return u
}

func (f *apiKeyRouterFixture) do(method, path, authz string, body any) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, req)
	return w
}

func TestAPIKeyRoutes_OwnerMintsAndServiceExchanges(t *testing.T) {
	f := newAPIKeyRouterFixture(t)
	owner := "Bearer " + f.tokens["owner"]

	w := f.do(http.MethodPost, "/api/v1/auth/users/svc-order/api-keys", owner,
		map[string]any{"name": "checkout", "permissions": []string{permissions.PromotionRedeem}, "expires_in_days": 90})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID        string     `json:"id"`
		APIKey    string     `json:"api_key"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if !domain.ValidAPIKeyFormat(created.APIKey) || created.ExpiresAt == nil {
		t.Fatalf("create response: %s", w.Body.String())
	}

	w = f.do(http.MethodGet, "/api/v1/auth/users/svc-order/api-keys", owner, nil)
	if w.Code != http.StatusOK || bytes.Contains(w.Body.Bytes(), []byte(`"api_key"`)) ||
		bytes.Contains(w.Body.Bytes(), []byte(created.APIKey)) {
		t.Fatalf("list must never carry the plaintext: %d %s", w.Code, w.Body.String())
	}

	// The exchange takes no bearer token — the key is the credential.
	w = f.do(http.MethodPost, "/api/v1/auth/token", "ApiKey "+created.APIKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
	}
	var tok struct {
		Token     string `json:"token"`
		TokenType string `json:"token_type"`
		ExpiresIn int    `json:"expires_in"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	if tok.TokenType != "Bearer" || tok.ExpiresIn != 900 || bytes.Contains(w.Body.Bytes(), []byte("refresh_token")) {
		t.Fatalf("exchange response: %s", w.Body.String())
	}
	parsed, err := jwt.Parse(tok.Token, func(*jwt.Token) (any, error) { return []byte(f.secret), nil })
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := parsed.Claims.(jwt.MapClaims)
	if c["sub"] != "svc-order" || c["type"] != "access" || c["account_type"] != "service" ||
		c["service_name"] != "dupli1-order" || c["token_use"] != "api_key" || c["akid"] != created.ID {
		t.Fatalf("claims = %v", c)
	}
	if perms, _ := c["permissions"].([]any); len(perms) != 1 || perms[0] != permissions.PromotionRedeem {
		t.Fatalf("permissions claim = %v, want the key's scope", c["permissions"])
	}

	// Revoke, then the key no longer exchanges.
	if w = f.do(http.MethodDelete, "/api/v1/auth/api-keys/"+created.ID, owner, nil); w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if w = f.do(http.MethodPost, "/api/v1/auth/token", "ApiKey "+created.APIKey, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key exchanged: %d", w.Code)
	}
}

func TestAPIKeyRoutes_ExchangeFailuresAreUniform(t *testing.T) {
	f := newAPIKeyRouterFixture(t)
	unknown, _ := domain.GenerateAPIKey(domain.APIKeyEnvLive)
	for _, authz := range []string{"", "Bearer " + f.tokens["owner"], "ApiKey ", "ApiKey not-a-key", "ApiKey " + unknown} {
		w := f.do(http.MethodPost, "/api/v1/auth/token", authz, nil)
		if w.Code != http.StatusUnauthorized || w.Body.String() != `{"error":"invalid_api_key"}` {
			t.Errorf("%q: %d %s", authz, w.Code, w.Body.String())
		}
	}
}

func TestAPIKeyRoutes_AccessRules(t *testing.T) {
	f := newAPIKeyRouterFixture(t)
	cases := []struct {
		name, role, method, path string
		want                     int
	}{
		{"admin cannot reach service accounts", "admin", http.MethodGet, "/api/v1/auth/users/svc-order/api-keys", http.StatusForbidden},
		{"admin cannot mint for them", "admin", http.MethodPost, "/api/v1/auth/users/svc-order/api-keys", http.StatusForbidden},
		{"no apikey permission", "manager", http.MethodGet, "/api/v1/auth/users/svc-order/api-keys", http.StatusForbidden},
		{"keys are for service accounts only", "owner", http.MethodPost, "/api/v1/auth/users/cust/api-keys", http.StatusBadRequest},
		{"unknown account", "owner", http.MethodGet, "/api/v1/auth/users/ghost/api-keys", http.StatusNotFound},
		{"unknown key", "owner", http.MethodDelete, "/api/v1/auth/api-keys/nope", http.StatusNotFound},
		{"unauthenticated", "", http.MethodGet, "/api/v1/auth/users/svc-order/api-keys", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		authz := ""
		if tc.role != "" {
			authz = "Bearer " + f.tokens[tc.role]
		}
		if w := f.do(tc.method, tc.path, authz, map[string]any{"name": "k"}); w.Code != tc.want {
			t.Errorf("%s: %d %s, want %d", tc.name, w.Code, w.Body.String(), tc.want)
		}
	}

	owner := "Bearer " + f.tokens["owner"]
	if w := f.do(http.MethodPost, "/api/v1/auth/users/svc-order/api-keys", owner,
		map[string]any{"name": "k", "permissions": []string{permissions.PaymentCancel}}); w.Code != http.StatusBadRequest {
		t.Errorf("scope beyond the account: %d %s", w.Code, w.Body.String())
	}

	// An env-seeded key cannot be revoked through the API: it would come back.
	envKey, _ := domain.GenerateAPIKey(domain.APIKeyEnvLive)
	if _, err := f.svc.SyncEnvAPIKey(t.Context(), "svc-order", envKey); err != nil {
		t.Fatal(err)
	}
	w := f.do(http.MethodGet, "/api/v1/auth/users/svc-order/api-keys", owner, nil)
	var list struct {
		Keys []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"api_keys"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Keys) != 1 || list.Keys[0].Source != "env" {
		t.Fatalf("list = %s", w.Body.String())
	}
	if w = f.do(http.MethodDelete, "/api/v1/auth/api-keys/"+list.Keys[0].ID, owner, nil); w.Code != http.StatusConflict {
		t.Fatalf("revoke env key: %d %s, want 409", w.Code, w.Body.String())
	}
}

func TestServiceAccountPasswordRoutes(t *testing.T) {
	f := newAPIKeyRouterFixture(t)
	owner := "Bearer " + f.tokens["owner"]

	if w := f.do(http.MethodPatch, "/api/v1/auth/users/svc-order/password", owner,
		map[string]string{"password": "brand-new-password"}); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("setting a service account's password: %d %s, want 422", w.Code, w.Body.String())
	}
	if w := f.do(http.MethodPost, "/api/v1/auth/register", owner,
		map[string]string{"email": "bot@example.com", "password": "some-password", "account_type": "service"}); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("registering a service account with a password: %d %s, want 422", w.Code, w.Body.String())
	}
	if w := f.do(http.MethodPost, "/api/v1/auth/register", owner,
		map[string]string{"email": "bot@example.com", "account_type": "service"}); w.Code != http.StatusCreated {
		t.Fatalf("registering a service account without one: %d %s, want 201", w.Code, w.Body.String())
	}
	if w := f.do(http.MethodPost, "/api/v1/auth/register", owner,
		map[string]string{"email": "person@example.com", "account_type": "manager"}); w.Code != http.StatusBadRequest {
		t.Fatalf("a person still needs a password: %d %s, want 400", w.Code, w.Body.String())
	}
}
