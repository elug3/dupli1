package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/support/pkg/domain"
)

func TestCallbackDataRoundTrips(t *testing.T) {
	for _, item := range domain.RootMenu {
		data := domain.CallbackData(item.Node)
		if len(data) > 64 {
			t.Fatalf("callback_data %q is %d bytes, over Telegram's 64-byte limit", data, len(data))
		}
		if got := domain.ParseCallbackData(data); got != item.Node {
			t.Fatalf("ParseCallbackData(%q) = %q, want %q", data, got, item.Node)
		}
	}
}

func TestStaleOrMalformedCallbackDataFallsBackToRoot(t *testing.T) {
	// Telegram keeps old messages tappable forever, so every one of these is a
	// tap a real shopper can still make.
	for _, data := range []string{"", "v0:ord", "ord", "v1:", "v1:nope", "garbage", "v1:ord:extra"} {
		if got := domain.ParseCallbackData(data); got != domain.NodeRoot {
			t.Fatalf("ParseCallbackData(%q) = %q, want the root menu", data, got)
		}
	}
}

func TestRootMenuCoversEveryTopic(t *testing.T) {
	if len(domain.RootMenu) != 5 {
		t.Fatalf("root menu has %d items, want the five consultation topics", len(domain.RootMenu))
	}
	seen := map[string]bool{}
	for _, item := range domain.RootMenu {
		if item.Label == "" {
			t.Fatalf("menu item %q has no label", item.Node)
		}
		if !domain.IsKnownNode(item.Node) {
			t.Fatalf("menu item %q is not a known node", item.Node)
		}
		if seen[item.Node] {
			t.Fatalf("node %q appears twice", item.Node)
		}
		seen[item.Node] = true
	}
}
