package irctext

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	tests := map[string]string{
		"plain title":            "plain title",
		"  padded  ":             "padded",
		"two\nlines":             "two lines",
		"tabs\tand   spaces":     "tabs and spaces",
		"colour \x03,04codes":    "colour ,04codes",
		"a\r\nQUIT":              "a QUIT",
		"‮en":                    "en",
		"nbsp separates":         "nbsp separates",
		"invalid \xff byte":      "invalid byte",
		"carriage\r\nPRIVMSG #c": "carriage PRIVMSG #c",
		"":                       "",
	}
	for in, want := range tests {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLineBudgetLeavesRoomForTheProtocol(t *testing.T) {
	for _, target := range []string{"#c", "#a-fairly-long-channel-name", "someone"} {
		budget := LineBudget(target)
		line := "PRIVMSG " + target + " :" + strings.Repeat("@", budget) + "\r\n"
		if len(line)+SourceReserve != LineLimit {
			t.Errorf("%q: a full line plus its source is %d bytes, want exactly %d",
				target, len(line)+SourceReserve, LineLimit)
		}
	}
}

// The line the server relays carries a source prefix the bot never wrote, and
// that is the line that has to fit.
func TestLineBudgetSurvivesTheRelay(t *testing.T) {
	const target = "#a-fairly-long-channel-name"
	source := ":" + strings.Repeat("n", 30) + "!" + strings.Repeat("u", 10) +
		"@" + strings.Repeat("h", 63) + " "

	relayed := source + "PRIVMSG " + target + " :" + strings.Repeat("@", LineBudget(target)) + "\r\n"
	if len(relayed) > LineLimit {
		t.Errorf("the relayed line is %d bytes, over the %d limit", len(relayed), LineLimit)
	}
}

func TestFitKeepsTheLineLegal(t *testing.T) {
	long := strings.Repeat("か", 400) // three bytes each, so far past the limit
	got := Fit("#chan", long)

	if n := len("PRIVMSG #chan :" + got + "\r\n"); n > LineLimit {
		t.Errorf("line is %d bytes, want at most %d", n, LineLimit)
	}
	if !strings.HasSuffix(got, ellipsis) {
		t.Errorf("a trimmed line should end in an ellipsis, got %q", got)
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("Fit split a rune: %q", got)
	}
}

func TestFitLeavesAShortLineAlone(t *testing.T) {
	msg := "YouTube: something short (Someone)"
	if got := Fit("#chan", msg); got != msg {
		t.Errorf("Fit trimmed a short line: %q", got)
	}
}

func TestFitGivesUpOnAnImpossibleTarget(t *testing.T) {
	if got := Fit(strings.Repeat("#", LineLimit), "anything"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTruncateCountsRunesNotBytes(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{strings.Repeat("é", 80), 64, strings.Repeat("é", 64)},
		{"short", 64, "short"},
		{"exact", 5, "exact"},
		{"anything", 0, ""},
		{"anything", -1, ""},
	}
	for _, tc := range tests {
		if got := Truncate(tc.in, tc.n); got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
