package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/bootstrap"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/handler"
	jwtgen "github.com/elug3/dupli1/auth/pkg/infra/jwt"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/auth/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type registrationStatsStub struct {
	times   []time.Time
	undated int
}

func (s *registrationStatsStub) RegistrationTimes(_ context.Context, accountType string, _, _ time.Time) ([]time.Time, int, error) {
	if accountType != domain.AccountTypeCustomer {
		return nil, 0, nil
	}
	return s.times, s.undated, nil
}

func newRegistrationReportRouter(t *testing.T, stats *registrationStatsStub) (*gin.Engine, *jwtgen.TokenGenerator, *fakeUserRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newFakeUserRepo()
	accessGen := jwtgen.NewTokenGeneratorWithType("access-secret", 900, "access")
	svc := service.NewService(repo, accessGen, service.WithRegistrationStats(stats))
	r := bootstrap.NewRouter(handler.NewHandler(svc, zerolog.Nop()), false, nil, nil, nil)
	return r, accessGen, repo
}

func TestRegistrationReport_ForbidsCustomer(t *testing.T) {
	r, accessGen, repo := newRegistrationReportRouter(t, &registrationStatsStub{})
	customer, err := domain.NewUser(uuid.New().String(), "cust@example.com", "secret", domain.AccountTypeCustomer)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(t.Context(), customer); err != nil {
		t.Fatal(err)
	}
	token, err := accessGen.Generate(t.Context(), customer.ID, customer.Permissions, ports.Identity{Email: customer.Email})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/reports/registrations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestRegistrationReport_CountsSignUpsPerWeek(t *testing.T) {
	stats := &registrationStatsStub{
		times:   []time.Time{time.Date(2026, 9, 30, 12, 0, 0, 0, reportperiod.Location)},
		undated: 2,
	}
	r, accessGen, repo := newRegistrationReportRouter(t, stats)
	manager, err := domain.NewUser(
		uuid.New().String(),
		"reports@internal.dupli1",
		"secret",
		domain.AccountTypeManager,
		permissions.UserRead,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(t.Context(), manager); err != nil {
		t.Fatal(err)
	}
	token, err := accessGen.Generate(t.Context(), manager.ID, manager.Permissions, ports.Identity{Email: manager.Email})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/reports/registrations?granularity=week&from=2026-09-28&to=2026-10-04", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var rep domain.RegistrationReport
	if err := json.NewDecoder(w.Body).Decode(&rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Periods) != 1 || rep.Periods[0].NewCustomers != 1 {
		t.Fatalf("periods = %+v, want one KST week with one sign-up", rep.Periods)
	}
	if rep.UndatedCustomers != 2 {
		t.Fatalf("undated = %d, want 2", rep.UndatedCustomers)
	}
}

func TestRegistrationReport_RejectsBadGranularity(t *testing.T) {
	r, accessGen, repo := newRegistrationReportRouter(t, &registrationStatsStub{})
	manager, err := domain.NewUser(
		uuid.New().String(),
		"reports@internal.dupli1",
		"secret",
		domain.AccountTypeManager,
		permissions.UserRead,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(t.Context(), manager); err != nil {
		t.Fatal(err)
	}
	token, err := accessGen.Generate(t.Context(), manager.ID, manager.Permissions, ports.Identity{Email: manager.Email})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/reports/registrations?granularity=day", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}
