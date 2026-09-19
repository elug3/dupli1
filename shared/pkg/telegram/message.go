package telegram

import (
	"strings"
	"unicode/utf8"
)

const (
	// maxMessageRunes is Telegram's sendMessage limit. A longer message is
	// rejected with a 400 and, since core NATS does not redeliver, the alert is
	// simply lost — so an order with enough line items used to go unreported.
	maxMessageRunes  = 4096
	truncationNotice = "\n… 생략됨"
)

// truncateMessage shortens message to Telegram's limit while keeping the HTML
// it parses valid: it never cuts inside a tag or an entity, and closes the tags
// the cut left open. Messages within the limit are returned untouched.
func truncateMessage(message string) string {
	if utf8.RuneCountInString(message) <= maxMessageRunes {
		return message
	}

	budget := maxMessageRunes - utf8.RuneCountInString(truncationNotice)
	// Closing tags cost characters of their own, so a first cut can still
	// overshoot; paying for them shrinks the budget and the next pass fits.
	for attempt := 0; attempt < 3 && budget > 0; attempt++ {
		cut := trimPartialMarkup(firstRunes(message, budget))
		closers := closingTags(cut)
		out := cut + closers + truncationNotice
		if utf8.RuneCountInString(out) <= maxMessageRunes {
			return out
		}
		budget -= utf8.RuneCountInString(closers)
	}

	// Nesting deep enough to defeat the loop: drop the notice and the closers
	// and send a plain, valid prefix.
	return trimPartialMarkup(firstRunes(message, maxMessageRunes))
}

func firstRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// trimPartialMarkup drops a trailing fragment of a tag or an entity, either of
// which Telegram rejects as malformed HTML.
func trimPartialMarkup(s string) string {
	if lt := strings.LastIndexByte(s, '<'); lt >= 0 && !strings.ContainsRune(s[lt:], '>') {
		s = s[:lt]
	}
	if amp := strings.LastIndexByte(s, '&'); amp >= 0 && !strings.ContainsRune(s[amp:], ';') {
		s = s[:amp]
	}
	return s
}

// closingTags returns the closers for every tag s leaves open, innermost first.
func closingTags(s string) string {
	var open []string
	for i := 0; i < len(s); {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			break
		}
		lt += i
		gt := strings.IndexByte(s[lt:], '>')
		if gt < 0 {
			break
		}
		gt += lt

		tag := s[lt+1 : gt]
		i = gt + 1
		if strings.HasSuffix(tag, "/") {
			continue // self-closing
		}
		if strings.HasPrefix(tag, "/") {
			name := strings.ToLower(strings.TrimPrefix(tag, "/"))
			for j := len(open) - 1; j >= 0; j-- {
				if open[j] == name {
					open = append(open[:j], open[j+1:]...)
					break
				}
			}
			continue
		}
		name := strings.ToLower(tag)
		if space := strings.IndexAny(name, " \t\n"); space >= 0 {
			name = name[:space] // <a href="…"> → a
		}
		if name != "" {
			open = append(open, name)
		}
	}

	var b strings.Builder
	for i := len(open) - 1; i >= 0; i-- {
		b.WriteString("</")
		b.WriteString(open[i])
		b.WriteString(">")
	}
	return b.String()
}
