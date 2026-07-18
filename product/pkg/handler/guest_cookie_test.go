package handler_test

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/oklog/ulid/v2"
)

func TestValidGuestID(t *testing.T) {
	good := ulid.Make().String()
	cases := []struct {
		id   string
		want bool
	}{
		{good, true},
		{"", false},
		{"short", false},
		{"not-a-ulid-!!!!!!!!!!!!!!", false},
		{good + "X", false},
		{"!!!!!!!!!!!!!!!!!!!!!!!!!!", false},
	}
	for _, tc := range cases {
		if got := handler.ValidGuestID(tc.id); got != tc.want {
			t.Fatalf("ValidGuestID(%q)=%v want %v", tc.id, got, tc.want)
		}
	}
}
