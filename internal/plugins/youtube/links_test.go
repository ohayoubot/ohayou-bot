package youtube

import (
	"reflect"
	"testing"
)

func TestVideoIDs(t *testing.T) {
	const id = "dQw4w9WgXcQ"

	tests := []struct {
		name string
		text string
		want []string
	}{
		{"watch", "https://www.youtube.com/watch?v=" + id, []string{id}},
		{"watch with extras", "https://youtube.com/watch?v=" + id + "&t=42s", []string{id}},
		{"short", "https://youtu.be/" + id, []string{id}},
		{"short with query", "http://youtu.be/" + id + "?t=42", []string{id}},
		{"shorts", "https://www.youtube.com/shorts/" + id, []string{id}},
		{"live", "https://www.youtube.com/live/" + id, []string{id}},
		{"embed", "https://www.youtube-nocookie.com/embed/" + id, []string{id}},
		{"mobile", "https://m.youtube.com/watch?v=" + id, []string{id}},
		{"music", "https://music.youtube.com/watch?v=" + id, []string{id}},
		{"mid sentence", "look at https://youtu.be/" + id + ", it's great", []string{id}},
		{"trailing stop", "watch https://youtu.be/" + id + ".", []string{id}},
		{"in parens", "(https://youtu.be/" + id + ")", []string{id}},
		{"upper case host", "HTTPS://YouTube.com/watch?v=" + id, []string{id}},

		{"no link", "nothing to see here", nil},
		{"not youtube", "https://example.com/watch?v=" + id, nil},
		{"lookalike host", "https://notyoutube.com/watch?v=" + id, nil},
		{"channel page", "https://www.youtube.com/@RickAstleyYT", nil},
		{"bare watch", "https://www.youtube.com/watch", nil},
		{"short id", "https://youtu.be/abc", nil},
		{"long id", "https://youtu.be/" + id + "xxxxx", nil},
		{"bad characters", "https://youtu.be/abcdefghij!", nil},
		{"no scheme", "youtu.be/" + id, nil},

		{"two links", "https://youtu.be/" + id + " and https://youtu.be/oHg5SJYRHA0",
			[]string{id, "oHg5SJYRHA0"}},
		{"same link twice", "https://youtu.be/" + id + " https://www.youtube.com/watch?v=" + id,
			[]string{id}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := videoIDs(tc.text, 4)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("videoIDs(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestVideoIDsRespectsMax(t *testing.T) {
	text := "https://youtu.be/dQw4w9WgXcQ https://youtu.be/oHg5SJYRHA0 https://youtu.be/QH2-TGUlwu4"
	got := videoIDs(text, 2)
	if len(got) != 2 {
		t.Fatalf("videoIDs returned %d ids (%v), want 2", len(got), got)
	}
	if got[0] != "dQw4w9WgXcQ" || got[1] != "oHg5SJYRHA0" {
		t.Errorf("videoIDs kept the wrong two: %v", got)
	}
}
