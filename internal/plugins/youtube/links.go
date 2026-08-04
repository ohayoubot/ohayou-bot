package youtube

import (
	"net/url"
	"regexp"
	"strings"
)

// urlRE finds candidate links in a line
var urlRE = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"']+`)

// idRE is youtube's video id
var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// hosts are the domains a video id can be read from, with any "www." or "m."
// already stripped.
var hosts = map[string]bool{
	"youtube.com":          true,
	"youtu.be":             true,
	"youtube-nocookie.com": true,
}

// paths are the url prefixes that carry the id as the next segment.
var paths = []string{"shorts", "live", "embed", "v"}

// videoIDs returns the ids of the videos linked in text, in the order they
// appear, at most max of them and never the same one twice.
func videoIDs(text string, max int) []string {
	var out []string
	seen := map[string]bool{}

	for _, raw := range urlRE.FindAllString(text, -1) {
		id, ok := videoID(trimPunct(raw))
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == max {
			break
		}
	}
	return out
}

// videoID pulls the video id out of one link.
func videoID(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	host = strings.TrimPrefix(host, "music.")
	if !hosts[host] {
		return "", false
	}

	segments := strings.FieldsFunc(u.EscapedPath(), func(r rune) bool { return r == '/' })

	switch {
	// youtu.be/<id>
	case host == "youtu.be":
		if len(segments) == 0 {
			return "", false
		}
		return check(segments[0])

	// youtube.com/watch?v=<id>
	case len(segments) == 1 && strings.EqualFold(segments[0], "watch"):
		return check(u.Query().Get("v"))

	// youtube.com/shorts/<id> and friends
	case len(segments) >= 2:
		for _, p := range paths {
			if strings.EqualFold(segments[0], p) {
				return check(segments[1])
			}
		}
	}
	return "", false
}

func check(id string) (string, bool) {
	if !idRE.MatchString(id) {
		return "", false
	}
	return id, true
}

// trimPunct drops the sentence a link was written into: "watch youtu.be/x." is
// a link followed by a full stop, not a link ending in one.
func trimPunct(s string) string {
	return strings.TrimRight(s, ".,;:!?'\")]}>")
}
