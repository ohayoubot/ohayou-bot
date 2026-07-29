package deerkins

import (
	"sort"
	"strings"
)

// The kinskode format is one character per cell, rows separated by \n. ' ' is
// black, 'A'-'O' the other fifteen mIRC colours, '_' transparent. It is the
// same format the web app stores, so the two render identically.
const (
	maxRows     = 30
	maxCols     = 40
	transparent = '_'

	fill      = "@"
	colorCode = "\x03"
	resetCode = "\x0f"
	boldCode  = "\x02"
)

var ircColor = map[byte]string{
	' ': "01", 'A': "00", 'B': "02", 'C': "03", 'D': "04", 'E': "05",
	'F': "06", 'G': "07", 'H': "08", 'I': "09", 'J': "10", 'K': "11",
	'L': "12", 'M': "13", 'N': "14", 'O': "15",
}

// inverse pairs each colour with its opposite for the invert modifier.
var inverse = map[byte]byte{
	' ': 'A', 'A': ' ', 'B': 'H', 'C': 'M', 'D': 'J', 'E': 'K',
	'F': 'I', 'G': 'L', 'H': 'B', 'I': 'M', 'J': 'D', 'K': 'E',
	'L': 'H', 'M': 'I', 'N': 'O', 'O': 'N', '_': ' ',
}

// modifiers are the single letters a request may carry before the pipe.
// 'x' is not a transform; it rolls a random pile of the others.
var modifierOrder = []byte{'i', 'm', 'n', 'd', 'r', 'u', 's', 'f', 't', 'x'}

var modifierNames = map[byte]string{
	'i': "invert", 'm': "mirror", 'n': "unitinu", 'd': "divide",
	'r': "reverse", 'u': "upsidedown", 's': "square", 'f': "flip",
	't': "transpose", 'x': "x",
}

var transforms = map[string]func([]string) []string{
	"invert":     invert,
	"mirror":     func(rows []string) []string { return halves(rows, 0) },
	"unitinu":    func(rows []string) []string { return halves(rows, 1) },
	"divide":     func(rows []string) []string { return halves(rows, 2) },
	"reverse":    reverse,
	"upsidedown": upsideDown,
	"square":     square,
	"flip":       flip,
	"transpose":  transposeRows,
}

// randomNames is the pool 'x' draws from, in a fixed order so a seeded
// generator gives the same picture twice.
var randomNames = []string{
	"invert", "mirror", "unitinu", "divide", "reverse",
	"upsidedown", "square", "flip", "transpose",
}

const (
	xRounds  = 3
	xMinMods = 4
	xMaxMods = 10
)

// normalise turns stored kinskode into rows of known characters. Anything the
// palette doesn't cover becomes transparent, so a row is always ascii and
// safe to index by byte. Oversized art is cropped rather than rejected: the
// gallery holds rows written long before the current limits.
func normalise(kinskode string) []string {
	kinskode = strings.ReplaceAll(kinskode, "\r\n", "\n")
	kinskode = strings.ReplaceAll(kinskode, "\r", "\n")

	rows := strings.Split(strings.ToUpper(kinskode), "\n")
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		var b strings.Builder
		for i := 0; i < len(row) && b.Len() < maxCols; i++ {
			if _, ok := ircColor[row[i]]; ok {
				b.WriteByte(row[i])
			} else {
				b.WriteByte(transparent)
			}
		}
		out = append(out, b.String())
	}
	for len(out) > 0 && isBlank(out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

func isBlank(row string) bool {
	return strings.Trim(row, string(transparent)) == ""
}

// toIRC paints each row with mIRC colour codes. Runs of one colour collapse to
// a bare fill and the colour state resets per line. A row stops early rather
// than exceed budget bytes, which keeps the line under the 512 byte protocol
// limit however wide the art is.
func toIRC(rows []string, budget int) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		var b strings.Builder
		last := ""
		for i := 0; i < len(row); i++ {
			color, ok := ircColor[row[i]]
			if !ok {
				color = "00" // transparent paints white, as on the web
			}
			cell := fill
			if color != last {
				cell = colorCode + color + "," + color + fill
			}
			if b.Len()+len(cell)+len(resetCode) > budget {
				break
			}
			b.WriteString(cell)
			last = color
		}
		b.WriteString(resetCode)
		out = append(out, b.String())
	}
	return out
}

// clamp bounds the art after modifiers have run. flip turns columns into rows,
// so a wide drawing can come back taller than it went in.
func clamp(rows []string, maxLines int) []string {
	if maxLines > maxRows {
		maxLines = maxRows
	}
	if len(rows) > maxLines {
		rows = rows[:maxLines]
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = head(row, maxCols)
	}
	return out
}

// splitRequest cuts "mods|name" into its parts. Modifiers are deduplicated and
// sorted, and 'x' swallows everything else.
func splitRequest(request string) (mods []byte, name string) {
	if i := strings.IndexByte(request, '|'); i >= 0 {
		mods, name = parseMods(request[:i]), request[i+1:]
	} else {
		name = request
	}
	return mods, strings.TrimSpace(name)
}

func parseMods(raw string) []byte {
	seen := map[byte]bool{}
	var mods []byte
	for i := 0; i < len(raw); i++ {
		c := lowerByte(raw[i])
		if _, ok := modifierNames[c]; !ok || seen[c] {
			continue
		}
		if c == 'x' {
			return []byte{'x'}
		}
		seen[c] = true
		mods = append(mods, c)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i] < mods[j] })
	return mods
}

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// applyMods runs the requested modifiers and reports the names of the ones
// that actually ran, which is the only way to know what 'x' rolled.
func applyMods(rows []string, mods []byte, roll func(int) int) ([]string, []string) {
	if len(mods) == 1 && mods[0] == 'x' {
		return applyX(rows, roll)
	}
	var used []string
	for _, m := range mods {
		fn, ok := transforms[modifierNames[m]]
		if !ok {
			continue
		}
		rows = fn(rows)
		used = append(used, modifierNames[m])
	}
	return rows, used
}

// applyX rolls between xMinMods and xMaxMods modifiers, then flips a coin on
// whether to go round again, up to xRounds times.
func applyX(rows []string, roll func(int) int) ([]string, []string) {
	var used []string
	for round := 0; round < xRounds; round++ {
		n := xMinMods + roll(xMaxMods-xMinMods+1)
		for i := 0; i < n; i++ {
			name := randomNames[roll(len(randomNames))]
			rows = transforms[name](rows)
			used = append(used, name)
		}
		if roll(2) == 0 {
			break
		}
	}
	return rows, used
}

func invert(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		b := []byte(row)
		for j := range b {
			if c, ok := inverse[b[j]]; ok {
				b[j] = c
			}
		}
		out[i] = string(b)
	}
	return out
}

func reverse(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = reverseString(row)
	}
	return out
}

func upsideDown(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[len(rows)-1-i] = row
	}
	return out
}

// halves mirrors one half of every row onto the other. direction 0 folds the
// left half rightwards, 1 the right half leftwards, 2 swaps the two.
func halves(rows []string, direction int) []string {
	half := widest(rows) / 2
	if half < 1 {
		return rows
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		rev := reverseString(row)
		switch direction {
		case 0:
			out[i] = head(rev, half) + tail(row, half)
		case 1:
			out[i] = head(row, half) + tail(rev, half)
		default:
			out[i] = tail(rev, half) + head(row, half)
		}
	}
	return out
}

// square rotates every row about its centre and the rows about theirs.
func square(rows []string) []string {
	half := widest(rows) / 2
	if half < 1 {
		return rows
	}
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		shifted := tail(row, half) + head(row, half)
		if i < len(rows)/2 {
			out = append(out, shifted)
		} else {
			out = append([]string{shifted}, out...)
		}
	}
	return out
}

// flip turns columns into rows.
func flip(rows []string) []string {
	width := widest(rows)
	if width == 0 {
		return rows
	}
	out := make([]string, 0, width)
	for x := 0; x < width; x++ {
		b := make([]byte, len(rows))
		for y, row := range rows {
			if x < len(row) {
				b[y] = row[x]
			} else {
				b[y] = transparent
			}
		}
		out = append(out, string(b))
	}
	return out
}

// transposeRows shears the picture, rotating each row one cell further than
// the one above it. The shift counter restarts whenever it reaches the width of
// the row it is about to move, which is why ragged art shears unevenly. The web
// app never ported this one, so the original is the only definition of it.
func transposeRows(rows []string) []string {
	out := make([]string, len(rows))
	shift := 0
	for i, row := range rows {
		width := len(row)
		if shift == width {
			shift = 0
		}
		if shift == 0 || width == 0 {
			out[i] = row
			shift++
			continue
		}
		keep := width - shift
		if keep < 0 {
			keep = width + keep // a shift past the width eats into the end
		}
		out[i] = tail(row, width-shift) + head(row, keep)
		shift++
	}
	return out
}

func widest(rows []string) int {
	w := 0
	for _, row := range rows {
		if len(row) > w {
			w = len(row)
		}
	}
	return w
}

func reverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// head and tail slice without panicking on an index past the end, the way the
// web app's slice() does.
func head(s string, n int) string {
	if n >= len(s) {
		return s
	}
	if n < 0 {
		return ""
	}
	return s[:n]
}

func tail(s string, n int) string {
	if n >= len(s) {
		return ""
	}
	if n < 0 {
		return s
	}
	return s[n:]
}
