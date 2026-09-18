package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateMessageLeavesShortMessagesAlone(t *testing.T) {
	msg := "🛒 <b>New order</b> ORD-1\nTotal: <b>₩1,000</b>"
	if got := truncateMessage(msg); got != msg {
		t.Fatalf("message within the limit was modified:\n%q", got)
	}
}

func TestTruncateMessageFitsTheLimit(t *testing.T) {
	msg := "<b>Order</b>\n" + strings.Repeat("1× SOME-LONG-SKU-NAME, ", 500)
	got := truncateMessage(msg)
	if n := utf8.RuneCountInString(got); n > maxMessageRunes {
		t.Fatalf("truncated message is %d runes, limit is %d", n, maxMessageRunes)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected a truncation notice, got tail %q", got[len(got)-40:])
	}
}

// Cutting inside a tag or an entity yields malformed HTML, which Telegram
// rejects with a 400 — the very failure truncation exists to avoid.
func TestTruncateMessageNeverCutsMarkup(t *testing.T) {
	cases := map[string]string{
		"mid-tag":    strings.Repeat("a", maxMessageRunes-5) + "<b>bold</b>" + strings.Repeat("c", 100),
		"mid-entity": strings.Repeat("a", maxMessageRunes-3) + "&amp;" + strings.Repeat("c", 100),
		"mid-href":   strings.Repeat("a", maxMessageRunes-10) + `<a href="https://example.com/orders/1">link</a>`,
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			got := truncateMessage(msg)
			if n := utf8.RuneCountInString(got); n > maxMessageRunes {
				t.Fatalf("%d runes exceeds the limit", n)
			}
			if lt := strings.LastIndexByte(got, '<'); lt >= 0 && !strings.ContainsRune(got[lt:], '>') {
				t.Fatalf("truncation left an unterminated tag: %q", got[lt:])
			}
			if amp := strings.LastIndexByte(got, '&'); amp >= 0 && !strings.ContainsRune(got[amp:], ';') {
				t.Fatalf("truncation left an unterminated entity: %q", got[amp:])
			}
		})
	}
}

// An open <b> at the cut point has to be closed, or the rest of the message
// renders as markup Telegram cannot parse.
func TestTruncateMessageClosesOpenTags(t *testing.T) {
	msg := "<b>" + strings.Repeat("x", maxMessageRunes*2) + "</b>"
	got := truncateMessage(msg)
	if !strings.HasPrefix(got, "<b>") {
		t.Fatal("expected the opening tag to survive")
	}
	if strings.Count(got, "<b>") != strings.Count(got, "</b>") {
		t.Fatalf("unbalanced tags in %q…%q", got[:20], got[len(got)-20:])
	}
}

func TestClosingTagsHandlesNestingAndAttributes(t *testing.T) {
	cases := map[string]string{
		"<b>bold":                         "</b>",
		"<b>bold</b>":                     "",
		`<b>a<a href="https://x/y">label`: "</a></b>",
		"<b>a</b><code>c":                 "</code>",
		"plain text":                      "",
		"<b>a<i>b</i>":                    "</b>",
	}
	for in, want := range cases {
		if got := closingTags(in); got != want {
			t.Fatalf("closingTags(%q) = %q, want %q", in, got, want)
		}
	}
}

// Multi-byte characters must not be split: half a rune is invalid UTF-8 and
// Telegram rejects the request.
func TestTruncateMessageKeepsRunesIntact(t *testing.T) {
	got := truncateMessage(strings.Repeat("한", maxMessageRunes*2))
	if !utf8.ValidString(got) {
		t.Fatal("truncation produced invalid UTF-8")
	}
	if n := utf8.RuneCountInString(got); n > maxMessageRunes {
		t.Fatalf("%d runes exceeds the limit", n)
	}
}
