package domain_test

import (
	"strings"
	"testing"

	"github.com/elug3/dupli1/support/pkg/domain"
)

func TestEveryNodeIsReachableFromTheRoot(t *testing.T) {
	// A node nobody can reach is dead copy someone will keep editing.
	seen := map[string]bool{domain.NodeRoot: true}
	queue := []string{domain.NodeRoot}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, child := range domain.ChildrenOf(id) {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			queue = append(queue, child.ID)
		}
	}
	for id := range domain.Nodes {
		if !seen[id] {
			t.Fatalf("node %q cannot be reached from the root menu", id)
		}
	}
}

func TestEveryNodeButTheRootHasALabelAndCopy(t *testing.T) {
	for id, node := range domain.Nodes {
		if id == domain.NodeRoot {
			continue
		}
		if strings.TrimSpace(node.Label) == "" {
			t.Fatalf("node %q has no button label", id)
		}
		if strings.TrimSpace(domain.AnswerFor(id)) == "" {
			t.Fatalf("node %q opens with no copy — Telegram rejects an empty message", id)
		}
	}
}

func TestEveryChildIsADefinedNode(t *testing.T) {
	for id, node := range domain.Nodes {
		for _, child := range node.Children {
			if !domain.IsKnownNode(child) {
				t.Fatalf("node %q lists child %q, which has no definition", id, child)
			}
		}
	}
}

func TestCallbackDataRoundTripsAndFitsTheLimit(t *testing.T) {
	for id := range domain.Nodes {
		data := domain.CallbackData(id)
		if len(data) > 64 {
			t.Fatalf("callback_data %q is %d bytes, over Telegram's 64-byte limit", data, len(data))
		}
		if got := domain.ParseCallbackData(data); got != id {
			t.Fatalf("ParseCallbackData(%q) = %q, want %q", data, got, id)
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

func TestRootMenuCoversTheFiveTopics(t *testing.T) {
	if got := len(domain.RootMenu()); got != 5 {
		t.Fatalf("root menu has %d items, want the five consultation topics", got)
	}
}

func TestSeededCopyNeverPromisesADay(t *testing.T) {
	// There is no holiday calendar, so a named day is a guess: "내일" said on
	// the eve of Chuseok is wrong by four days. The copy states the window.
	forbidden := []string{"내일", "오늘 중", "24시간 이내", "곧 답변"}
	for node, body := range domain.DefaultAnswers {
		for _, phrase := range forbidden {
			if strings.Contains(body, phrase) {
				t.Fatalf("node %q promises a time (%q); state the service window instead", node, phrase)
			}
		}
	}
}
