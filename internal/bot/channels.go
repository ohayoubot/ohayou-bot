package bot

import "strings"

// SharedWith returns the channels both the bot and someone else are in, spelled
// the way the bot has them: the name travels into grants and queued lines, so it
// should be the one the bot joined and not the one WHOIS echoed back.
//
// Repeats and channels the bot is not in are dropped.
func (b *Bot) SharedWith(theirs []string) []string {
	mine := map[string]string{}
	for _, name := range b.Channels() {
		mine[strings.ToLower(name)] = name
	}

	var out []string
	seen := map[string]bool{}
	for _, name := range theirs {
		key := strings.ToLower(name)
		canonical, ok := mine[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, canonical)
	}
	return out
}
