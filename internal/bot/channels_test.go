package bot

import (
	"strings"
	"testing"
)

func TestSharedWithKeepsTheBotsSpelling(t *testing.T) {
	b := testBot()
	b.addChannel("#chan")
	b.addChannel("#Other")

	// #nowhere is not somewhere the bot is, #CHAN is a repeat in another case,
	// and #Other comes back the way the bot has it rather than the way this was
	// spelled.
	got := b.SharedWith([]string{"#chan", "#CHAN", "#nowhere", "#other"})
	if strings.Join(got, " ") != "#chan #Other" {
		t.Errorf("SharedWith = %v, want [#chan #Other]", got)
	}
}

func TestSharedWithNothingInCommon(t *testing.T) {
	b := testBot()
	b.addChannel("#chan")

	if got := b.SharedWith([]string{"#elsewhere"}); len(got) != 0 {
		t.Errorf("SharedWith = %v, want nothing", got)
	}
	if got := b.SharedWith(nil); len(got) != 0 {
		t.Errorf("SharedWith(nil) = %v, want nothing", got)
	}
}
