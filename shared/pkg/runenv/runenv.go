// Package runenv says whether a process runs as development or production,
// from DUPLI1_ENV, and holds production to explicit configuration.
//
//	DUPLI1_ENV=dev    local development: in-memory fallbacks, an ephemeral
//	                  signing key and the published dev credentials are allowed
//	DUPLI1_ENV=prod   production (also the default when DUPLI1_ENV is unset)
//
// Unset means prod, so a forgotten variable can never switch dev defaults on
// in production; the local docker-compose.yml sets dev explicitly. In prod each
// service refuses to start, naming every problem at once, when something it
// would otherwise fall back on is missing (a database URL, AUTH_JWKS_URL,
// NATS_TOKEN, a signing key) or still holds a value published in this
// repository for local development (DevValues).
package runenv

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Var is the environment variable that selects the mode.
const Var = "DUPLI1_ENV"

// Mode is the runtime environment.
type Mode string

const (
	Dev  Mode = "dev"
	Prod Mode = "prod"
)

// DevValues are the credentials the repository publishes for local
// development (docker-compose.yml, .env.example files). None of them may
// reach production.
var DevValues = []string{
	"password",
	"dupli1_dev",
	"dupli1_nats_dev",
	"dev-jwt-secret-do-not-use-in-production",
}

// devPrefixes mark values that are dev-only by shape: a "test" service API
// key, or a .env.prod.example placeholder left unfilled.
var devPrefixes = []string{"dk_test_", "change-me"}

// Current reads DUPLI1_ENV. Blank means Prod; anything other than dev or prod
// is an error, so a typo cannot pick a mode by accident.
func Current() (Mode, error) {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv(Var))); v {
	case "", string(Prod):
		return Prod, nil
	case string(Dev):
		return Dev, nil
	default:
		return "", fmt.Errorf("%s must be %q or %q, got %q", Var, Dev, Prod, v)
	}
}

// IsDevValue reports whether v is one of DevValues or has a dev-only prefix.
func IsDevValue(v string) bool {
	v = strings.TrimSpace(v)
	for _, d := range DevValues {
		if v == d {
			return true
		}
	}
	for _, p := range devPrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// Checks collects what a production process is missing. Every method records
// a problem rather than stopping, so one start-up names all of them.
type Checks struct {
	problems []string
}

// Require records a problem when value is blank.
func (c *Checks) Require(name, value string) {
	if strings.TrimSpace(value) == "" {
		c.Failf("%s is required", name)
	}
}

// NotDev records a problem when value is set to a dev credential. A blank
// value passes; pair it with Require when the setting is mandatory.
func (c *Checks) NotDev(name, value string) {
	if strings.TrimSpace(value) != "" && IsDevValue(value) {
		c.Failf("%s is set to a development value", name)
	}
}

// Secret requires value and refuses a dev credential.
func (c *Checks) Secret(name, value string) {
	c.Require(name, value)
	c.NotDev(name, value)
}

// URL requires a connection URL and refuses one whose password is a dev
// credential (postgres://dupli1:dupli1_dev@…).
func (c *Checks) URL(name, raw string) {
	c.Require(name, raw)
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok && IsDevValue(pw) {
			c.Failf("%s carries a development password", name)
		}
	}
}

// Failf records a problem.
func (c *Checks) Failf(format string, args ...any) {
	c.problems = append(c.problems, fmt.Sprintf(format, args...))
}

// Err returns every recorded problem as one error, or nil.
func (c *Checks) Err() error {
	if len(c.problems) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to start in production (%s unset or prod): %s; set %s=dev for local development",
		Var, strings.Join(c.problems, "; "), Var)
}

// Enforce resolves the mode and, in Prod, runs check and returns its problems.
// In Dev it skips check and returns nil. An invalid DUPLI1_ENV is an error in
// either case.
func Enforce(check func(*Checks)) (Mode, error) {
	mode, err := Current()
	if err != nil {
		return "", err
	}
	if mode == Dev {
		return Dev, nil
	}
	var c Checks
	check(&c)
	return Prod, c.Err()
}
