package permissions

import (
	"fmt"
	"strings"
)

// Valid reports whether s is a known concrete permission or an allowed wildcard
// (*, admin.*, or {resource}.*).
func Valid(s string) bool {
	if s == "" {
		return false
	}
	if s == All {
		return true
	}
	if strings.HasSuffix(s, ".*") {
		prefix := strings.TrimSuffix(s, ".*")
		if prefix == "" {
			return false
		}
		for _, r := range prefix {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
		return true
	}
	return IsKnown(s)
}

// Validate returns an error if any entry is not a valid permission string.
func Validate(perms []string) error {
	for _, p := range perms {
		if !Valid(p) {
			return fmt.Errorf("invalid permission: %q", p)
		}
	}
	return nil
}

// Union returns a deduplicated merge of the given permission slices.
func Union(slices ...[]string) []string {
	var combined []string
	for _, s := range slices {
		combined = append(combined, s...)
	}
	return Dedupe(combined)
}

// Dedupe returns a copy of perms with duplicates removed, preserving first-seen order.
// An empty result is returned as a non-nil slice for JSON/DB compatibility.
func Dedupe(perms []string) []string {
	if len(perms) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(perms))
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}
