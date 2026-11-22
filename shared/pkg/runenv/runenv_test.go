package runenv

import (
	"strings"
	"testing"
)

func TestCurrent(t *testing.T) {
	for in, want := range map[string]Mode{"": Prod, "  ": Prod, "prod": Prod, "PROD": Prod, "dev": Dev, " Dev ": Dev} {
		t.Setenv(Var, in)
		got, err := Current()
		if err != nil || got != want {
			t.Errorf("Current() with %q = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"production", "development", "staging", "local"} {
		t.Setenv(Var, bad)
		if _, err := Current(); err == nil {
			t.Errorf("Current() with %q: want error", bad)
		}
	}
}

func TestEnforceDevSkipsChecks(t *testing.T) {
	t.Setenv(Var, "dev")
	ran := false
	mode, err := Enforce(func(c *Checks) { ran = true; c.Require("X", "") })
	if mode != Dev || err != nil || ran {
		t.Fatalf("Enforce in dev = %q, %v (check ran: %v); want dev, nil, not run", mode, err, ran)
	}
}

func TestEnforceProdNamesEveryProblem(t *testing.T) {
	t.Setenv(Var, "")
	mode, err := Enforce(func(c *Checks) {
		c.Require("DUPLI1_ORDER_DB", "")
		c.Secret("NATS_TOKEN", "dupli1_nats_dev")
		c.URL("DB_URL", "postgres://dupli1:dupli1_dev@postgres:5432/orders?sslmode=disable")
		c.NotDev("DUPLI1_ORDER_SERVICE_API_KEY", "dk_test_local-dev-order-service-key-not-a-secretxxx")
		c.NotDev("JWT_SECRET", "") // blank passes NotDev
		c.Secret("OWNER_PASSWORD", "s3cret-long-enough")
	})
	if mode != Prod || err == nil {
		t.Fatalf("Enforce in prod = %q, %v; want prod and an error", mode, err)
	}
	msg := err.Error()
	for _, want := range []string{
		"DUPLI1_ORDER_DB is required",
		"NATS_TOKEN is set to a development value",
		"DB_URL carries a development password",
		"DUPLI1_ORDER_SERVICE_API_KEY is set to a development value",
		"DUPLI1_ENV=dev",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	for _, unwanted := range []string{"JWT_SECRET", "OWNER_PASSWORD"} {
		if strings.Contains(msg, unwanted) {
			t.Errorf("error %q should not mention %s", msg, unwanted)
		}
	}
}

func TestIsDevValue(t *testing.T) {
	for _, v := range []string{"password", " dupli1_dev ", "dk_test_x", "change-me-strong-jwt-secret"} {
		if !IsDevValue(v) {
			t.Errorf("IsDevValue(%q) = false", v)
		}
	}
	for _, v := range []string{"", "dk_live_x", "Password1", "dupli1_dev_not"} {
		if IsDevValue(v) {
			t.Errorf("IsDevValue(%q) = true", v)
		}
	}
}

func TestEnforceProdPassesWhenConfigured(t *testing.T) {
	t.Setenv(Var, "prod")
	_, err := Enforce(func(c *Checks) {
		c.URL("DB_URL", "postgres://schick:Zx9-random@postgres:5432/orders")
		c.Secret("NATS_TOKEN", "a-real-token")
		c.NotDev("DUPLI1_ORDER_SERVICE_API_KEY", "dk_live_abc")
	})
	if err != nil {
		t.Fatalf("Enforce = %v, want nil", err)
	}
}

func TestEnforceRejectsInvalidMode(t *testing.T) {
	t.Setenv(Var, "production")
	if _, err := Enforce(func(*Checks) {}); err == nil {
		t.Fatal("want error for DUPLI1_ENV=production")
	}
}
