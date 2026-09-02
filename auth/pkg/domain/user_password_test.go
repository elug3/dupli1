package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

func TestValidateDummyPassword_NeverMatches(t *testing.T) {
	// ValidateDummyPassword exists purely to burn comparable bcrypt time on
	// Login's "no such account" path; it must never report success, since it
	// has no real password to match.
	for _, pw := range []string{"", "whatever", "password12"} {
		if domain.ValidateDummyPassword(pw) {
			t.Fatalf("ValidateDummyPassword(%q) = true, want false", pw)
		}
	}
}
