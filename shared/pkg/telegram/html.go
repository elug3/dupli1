package telegram

import "strings"

// EscapeHTML escapes the characters Telegram's HTML parse mode treats as
// markup. Quotes are included because escaped values are also interpolated into
// attributes — an order ID inside href="…", for one — where an unescaped quote
// would end the attribute and let the rest of the value inject its own.
//
// Every bot formatting HTML messages needs this, and notification carried two
// identical copies of it (pkg/infra/telegram and pkg/service) with a comment on
// one asking that they be kept in step. One copy, here, is that request granted.
func EscapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}
