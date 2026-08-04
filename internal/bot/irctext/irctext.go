// Package irctext prepares text for sending to irc
package irctext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// LineLimit is the protocol's 512 bytes for a whole line, including the
// "PRIVMSG <target> :" the bot writes and the trailing CRLF.
const LineLimit = 512

const ellipsis = "..."

// LineBudget is how many bytes of message survive the trip to target as one
// line. It can be zero or negative for an absurdly long target, which callers
// are expected to notice.
func LineBudget(target string) int {
	return LineLimit - len("PRIVMSG "+target+" :\r\n")
}

// Fit trims msg to the target's budget, marking the cut with an ellipsis.
func Fit(target, msg string) string {
	budget := LineBudget(target)
	if budget < 1 {
		return ""
	}
	if len(msg) <= budget {
		return msg
	}

	cut := budget - len(ellipsis)
	if cut < 1 {
		return msg[:budget]
	}
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return strings.TrimRight(msg[:cut], " ") + ellipsis
}

// Clean flattens untrusted text into something safe to send
func Clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	space := false
	for _, r := range strings.ToValidUTF8(s, "") {
		switch {
		// Newlines and tabs are whitespace before they are control characters
		case unicode.IsSpace(r):
			space = b.Len() > 0
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		default:
			if space {
				b.WriteRune(' ')
				space = false
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Truncate cuts s to at most n runes.
func Truncate(s string, n int) string {
	if n < 0 {
		return ""
	}
	if len(s) <= n { // bytes bound runes, so a short string is always under
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
