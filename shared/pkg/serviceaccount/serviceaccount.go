// Package serviceaccount names the machine accounts that call internal APIs.
//
// Auth stamps a name on each seeded service account and carries it in the
// access token's service_name claim; internal routes allowlist names (see
// authjwt.Claims.CalledBy). One list, so the name a seed writes and the name
// a route checks cannot drift apart. No dependencies, so auth can import it
// without pulling in the JWKS validator.
package serviceaccount

// AccountType is the account_type claim value for machine accounts.
const AccountType = "service"

// Names of the seeded service accounts.
const (
	Order = "dupli1-order"
	Web   = "dupli1-web"
)
