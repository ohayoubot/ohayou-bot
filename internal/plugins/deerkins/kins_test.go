package deerkins

import (
	"strings"
	"testing"
)

func rows(lines ...string) []string { return lines }

func TestNormaliseKeepsPaletteAndDropsTheRest(t *testing.T) {
	// é is two bytes, and each one is a cell that isn't in the palette.
	got := normalise("ab_c\nDéx\n")
	want := rows("AB_C", "D___")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("normalise = %q, want %q", got, want)
	}
}

func TestNormaliseTrimsTrailingBlankRows(t *testing.T) {
	if got := normalise("A\n__\n\n"); len(got) != 1 || got[0] != "A" {
		t.Errorf("normalise = %q, want [A]", got)
	}
	if got := normalise("__\n__"); len(got) != 0 {
		t.Errorf("normalise(blank) = %q, want none", got)
	}
}

func TestNormaliseCrops(t *testing.T) {
	wide := strings.Repeat("A", 200)
	tall := strings.Repeat(wide+"\n", 100)
	got := normalise(tall)
	if len(got) != maxRows {
		t.Fatalf("rows = %d, want %d", len(got), maxRows)
	}
	for _, row := range got {
		if len(row) != maxCols {
			t.Fatalf("row width = %d, want %d", len(row), maxCols)
		}
	}
}

// The expected output here comes from the web app's own encoder, which paints
// the same art for the gallery.
func TestToIRCMatchesTheWebApp(t *testing.T) {
	got := toIRC(normalise("        GGGG\n A A    A A"), 10000)
	want := []string{
		"\x0301,01@@@@@@@@\x0307,07@@@@\x0f",
		"\x0301,01@\x0300,00@\x0301,01@\x0300,00@\x0301,01@@@@\x0300,00@\x0301,01@\x0300,00@\x0f",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToIRCPaintsTransparentAsWhite(t *testing.T) {
	if got := toIRC(rows("_A"), 10000)[0]; got != "\x0300,00@@\x0f" {
		t.Errorf("toIRC = %q", got)
	}
}

func TestToIRCStaysWithinBudget(t *testing.T) {
	budget := 40
	for _, line := range toIRC(normalise(" ABCDEFGHIJKLMNO ABCDEFGHIJKLMNO"), budget) {
		if len(line) > budget {
			t.Errorf("line is %d bytes, over the %d budget: %q", len(line), budget, line)
		}
		if !strings.HasSuffix(line, resetCode) {
			t.Errorf("line %q is not terminated", line)
		}
	}
}

func TestClampBoundsWhatGetsPainted(t *testing.T) {
	tall := make([]string, 50)
	for i := range tall {
		tall[i] = strings.Repeat("A", 60)
	}
	got := clamp(tall, 40)
	if len(got) != maxRows {
		t.Errorf("rows = %d, want %d", len(got), maxRows)
	}
	if len(got[0]) != maxCols {
		t.Errorf("cols = %d, want %d", len(got[0]), maxCols)
	}
	if got := clamp(tall, 3); len(got) != 3 {
		t.Errorf("rows = %d, want 3", len(got))
	}
}

func TestTransforms(t *testing.T) {
	art := rows("ABCD", "EFGH")
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"invert", rows(" AB_"), rows("A H ")},
		{"reverse", art, rows("DCBA", "HGFE")},
		{"upsidedown", art, rows("EFGH", "ABCD")},
		{"flip", art, rows("AE", "BF", "CG", "DH")},
		{"mirror", art, rows("DCCD", "HGGH")},
		{"unitinu", art, rows("ABBA", "EFFE")},
		{"divide", art, rows("BAAB", "FEEF")},
		{"square", art, rows("GHEF", "CDAB")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transforms[tt.name](tt.in)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestTransposeShearsWithARestartingShift(t *testing.T) {
	in := rows("ABCDE", "ABCDE", "ABCDE", "ABCDE", "ABCDE", "ABCDE", "ABC", "AB", "A")
	want := rows("ABCDE", "EABCD", "DEABC", "CDEAB", "BCDEA", "ABCDE", "CAB", "AB", "A")
	got := transposeRows(in)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("transpose = %q, want %q", got, want)
	}
}

func TestTransformsSurviveRaggedAndEmptyArt(t *testing.T) {
	ragged := rows("", "A", "ABCDEFG", "AB")
	for name, fn := range transforms {
		t.Run(name, func(t *testing.T) {
			fn(ragged)
			fn(nil)
			fn(rows(""))
		})
	}
}

func TestSplitRequest(t *testing.T) {
	tests := []struct {
		in       string
		wantMods string
		wantName string
	}{
		{"senordeer", "", "senordeer"},
		{"iu|senordeer", "iu", "senordeer"},
		{"ui|senordeer", "iu", "senordeer"},  // sorted
		{"uui|senordeer", "iu", "senordeer"}, // deduplicated
		{"uzq!i|deer", "iu", "deer"},         // unknown letters dropped
		{"ixu|senordeer", "x", "senordeer"},  // x swallows the rest
		{"|senordeer", "", "senordeer"},      // empty mods
		{"iu|", "iu", ""},                    // no name
		{"IU|SenorDeer", "iu", "SenorDeer"},  // case insensitive mods
		{"help modifiers", "", "help modifiers"},
	}
	for _, tt := range tests {
		mods, name := splitRequest(tt.in)
		if string(mods) != tt.wantMods || name != tt.wantName {
			t.Errorf("splitRequest(%q) = %q, %q; want %q, %q", tt.in, mods, name, tt.wantMods, tt.wantName)
		}
	}
}

func TestApplyModsReportsWhatItRan(t *testing.T) {
	got, used := applyMods(rows("ABCD"), []byte{'r', 'u'}, nil)
	if strings.Join(used, ",") != "reverse,upsidedown" {
		t.Errorf("used = %v", used)
	}
	if got[0] != "DCBA" {
		t.Errorf("rows = %q", got)
	}
}

func TestApplyXRollsBetweenFourAndThirtyModifiers(t *testing.T) {
	art := normalise("ABCD\nEFGH")
	for _, roll := range []func(int) int{
		func(int) int { return 0 },             // always the first choice, never repeats
		func(n int) int { return n - 1 },       // always the last choice, always repeats
		func(n int) int { return (n * 3) % 7 }, // something in between
	} {
		got, used := applyX(art, roll)
		if len(used) < xMinMods || len(used) > xRounds*xMaxMods {
			t.Errorf("x used %d modifiers, want %d-%d", len(used), xMinMods, xRounds*xMaxMods)
		}
		if len(got) == 0 {
			t.Error("x painted nothing")
		}
	}
}
