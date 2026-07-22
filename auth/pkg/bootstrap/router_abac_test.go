package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/handler"
	jwtgen "github.com/elug3/dupli1/auth/pkg/infra/jwt"
	"github.com/elug3/dupli1/auth/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func TestManagerCanManageCustomerPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRBACFakeRepo()
	accessGen := jwtgen.NewTokenGeneratorWithType("test-secret", 900, "access")
	refreshGen := jwtgen.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	svc := service.NewService(repo, accessGen, service.WithRefreshTokenGen(refreshGen, time.Hour))
	h := handler.NewHandler(svc, zerolog.Nop())
	r := newRouter(h, false, nil, nil, nil, settings.NewResponse("auth"))

	manager, _ := domain.NewUser(uuid.New().String(), "mgr@dupli1.com", "password", domain.AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)
	customer, _ := domain.NewUser(uuid.New().String(), "cust@example.com", "password", domain.AccountTypeCustomer)
	_ = repo.Save(context.Background(), manager)
	_ = repo.Save(context.Background(), customer)

	token, _ := accessGen.Generate(context.Background(), manager.ID, manager.Permissions)
	body, _ := json.Marshal(map[string]string{"password": "newsecret1"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/users/"+customer.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("manager patch customer password: want 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestManagerCannotManageAdminPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRBACFakeRepo()
	accessGen := jwtgen.NewTokenGeneratorWithType("test-secret", 900, "access")
	refreshGen := jwtgen.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	svc := service.NewService(repo, accessGen, service.WithRefreshTokenGen(refreshGen, time.Hour))
	h := handler.NewHandler(svc, zerolog.Nop())
	r := newRouter(h, false, nil, nil, nil, settings.NewResponse("auth"))

	manager, _ := domain.NewUser(uuid.New().String(), "mgr@dupli1.com", "password", domain.AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)
	admin, _ := domain.NewUser(uuid.New().String(), "admin@dupli1.com", "password", domain.AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	_ = repo.Save(context.Background(), manager)
	_ = repo.Save(context.Background(), admin)

	token, _ := accessGen.Generate(context.Background(), manager.ID, manager.Permissions)
	body, _ := json.Marshal(map[string]string{"password": "newsecret1"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/users/"+admin.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("manager patch admin password: want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCanManageManagerPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRBACFakeRepo()
	accessGen := jwtgen.NewTokenGeneratorWithType("test-secret", 900, "access")
	refreshGen := jwtgen.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	svc := service.NewService(repo, accessGen, service.WithRefreshTokenGen(refreshGen, time.Hour))
	h := handler.NewHandler(svc, zerolog.Nop())
	r := newRouter(h, false, nil, nil, nil, settings.NewResponse("auth"))

	admin, _ := domain.NewUser(uuid.New().String(), "admin@dupli1.com", "password", domain.AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	manager, _ := domain.NewUser(uuid.New().String(), "mgr@dupli1.com", "password", domain.AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)
	_ = repo.Save(context.Background(), admin)
	_ = repo.Save(context.Background(), manager)

	token, _ := accessGen.Generate(context.Background(), admin.ID, admin.Permissions)
	body, _ := json.Marshal(map[string]string{"password": "newsecret1"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/users/"+manager.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("admin patch manager password: want 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCannotManageOwnerPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRBACFakeRepo()
	accessGen := jwtgen.NewTokenGeneratorWithType("test-secret", 900, "access")
	refreshGen := jwtgen.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	svc := service.NewService(repo, accessGen, service.WithRefreshTokenGen(refreshGen, time.Hour))
	h := handler.NewHandler(svc, zerolog.Nop())
	r := newRouter(h, false, nil, nil, nil, settings.NewResponse("auth"))

	admin, _ := domain.NewUser(uuid.New().String(), "admin@dupli1.com", "password", domain.AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	owner, _ := domain.NewUser(uuid.New().String(), "owner@dupli1.com", "password", domain.AccountTypeManager, permissions.All)
	_ = repo.Save(context.Background(), admin)
	_ = repo.Save(context.Background(), owner)

	token, _ := accessGen.Generate(context.Background(), admin.ID, admin.Permissions)
	body, _ := json.Marshal(map[string]string{"password": "newsecret1"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/users/"+owner.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin patch owner password: want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOwnerCanManageAdminPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRBACFakeRepo()
	accessGen := jwtgen.NewTokenGeneratorWithType("test-secret", 900, "access")
	refreshGen := jwtgen.NewTokenGeneratorWithType("test-secret", 3600, "refresh")
	svc := service.NewService(repo, accessGen, service.WithRefreshTokenGen(refreshGen, time.Hour))
	h := handler.NewHandler(svc, zerolog.Nop())
	r := newRouter(h, false, nil, nil, nil, settings.NewResponse("auth"))

	owner, _ := domain.NewUser(uuid.New().String(), "owner@dupli1.com", "password", domain.AccountTypeManager, permissions.All)
	admin, _ := domain.NewUser(uuid.New().String(), "admin@dupli1.com", "password", domain.AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	_ = repo.Save(context.Background(), owner)
	_ = repo.Save(context.Background(), admin)

	token, _ := accessGen.Generate(context.Background(), owner.ID, owner.Permissions)
	body, _ := json.Marshal(map[string]string{"password": "newsecret1"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/users/"+admin.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("owner patch admin password: want 204, got %d: %s", w.Code, w.Body.String())
	}
}
