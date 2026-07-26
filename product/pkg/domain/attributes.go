package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxAttributeEntries = 32
	maxAttributeKeyLen  = 64
	maxAttributeValLen  = 512
)

// NormalizeAttributes trims keys/values, drops empty keys, and enforces size
// limits. Nil maps are left unchanged (merge treats nil as "omit").
func NormalizeAttributes(attrs map[string]string) (map[string]string, error) {
	if attrs == nil {
		return nil, nil
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if utf8.RuneCountInString(key) > maxAttributeKeyLen {
			return nil, fmt.Errorf("attribute key %q exceeds %d characters", key, maxAttributeKeyLen)
		}
		val := strings.TrimSpace(v)
		if utf8.RuneCountInString(val) > maxAttributeValLen {
			return nil, fmt.Errorf("attribute %q value exceeds %d characters", key, maxAttributeValLen)
		}
		out[key] = val
	}
	if len(out) > maxAttributeEntries {
		return nil, fmt.Errorf("attributes exceed %d entries", maxAttributeEntries)
	}
	return out, nil
}
