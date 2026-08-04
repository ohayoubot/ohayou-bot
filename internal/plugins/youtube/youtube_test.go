package youtube

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot/bottest"
	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

const testID = "dQw4w9WgXcQ"

// harness runs the plugin against a fake irc server and a fake oembed endpoint,
// so a test can put a line in a channel and read what comes back out.
type harness struct {
	*bottest.Harness
	plugin *Plugin

	mu     sync.Mutex
	asked  []string         // the video ids oembed was asked about
	videos map[string]video // what oembed knows
	status map[string]int   // ids that answer with an error instead
	clock  time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		Harness: bottest.New(t, bottest.Ignoring("spammer")),
		videos:  map[string]video{},
		status:  map[string]int{},
		clock:   time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}
	h.videos[testID] = video{
		Title:  "Rick Astley - Never Gonna Give You Up",
		Author: "Rick Astley",
	}

	oembed := httptest.NewServer(http.HandlerFunc(h.oembed))
	t.Cleanup(oembed.Close)

	h.plugin = New()
	h.plugin.now = h.now
	if _, err := h.plugin.Configure(plugin.Config{Block: json.RawMessage(`{
		"requestTimeout": 2000,
		"ignoreChannels": ["#quiet"]
	}`)}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	deps := plugin.Deps{Bot: h.Bot, Log: h.Log}
	if err := h.plugin.Register(deps.For("youtube")); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.plugin.api = newClient(oembed.URL, 2*time.Second)

	h.Start()
	return h
}

func (h *harness) now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clock
}

// advance moves the plugin's clock forward.
func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(d)
}

// knows teaches the fake endpoint about a video.
func (h *harness) knows(id string, v video) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.videos[id] = v
}

// fails makes the fake endpoint answer for an id with a status instead.
func (h *harness) fails(id string, code int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status[id] = code
}

// oembed answers as youtube's endpoint does, for the videos the test set up.
func (h *harness) oembed(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(r.URL.Query().Get("url"))
	id := ""
	if target != nil {
		id = target.Query().Get("v")
	}

	h.mu.Lock()
	h.asked = append(h.asked, id)
	v, known := h.videos[id]
	code, failing := h.status[id]
	h.mu.Unlock()

	switch {
	case failing:
		w.WriteHeader(code)
	case !known:
		w.WriteHeader(http.StatusNotFound)
	default:
		json.NewEncoder(w).Encode(v)
	}
}

func (h *harness) askedFor() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.asked...)
}

func TestPreviewsALink(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "#chan", "have you seen https://youtu.be/"+testID+" yet")

	want := "PRIVMSG #chan :YouTube: Rick Astley - Never Gonna Give You Up (Rick Astley)"
	if got := h.Next(); got != want {
		t.Errorf("bot said %q, want %q", got, want)
	}
}

func TestPreviewsInAPrivateMessage(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "ohayoubot", "https://www.youtube.com/watch?v="+testID)

	if got := h.Next(); !strings.HasPrefix(got, "PRIVMSG someone :YouTube: ") {
		t.Errorf("bot said %q, want a preview back to the sender", got)
	}
}

func TestIgnoresLinesWithoutAVideo(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "#chan", "https://example.com/watch?v="+testID+" is not youtube")
	h.Silent()

	if asked := h.askedFor(); len(asked) != 0 {
		t.Errorf("oembed was asked about %v, want nothing", asked)
	}
}

func TestSaysNothingAboutAMissingVideo(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "#chan", "https://youtu.be/oHg5SJYRHA0")
	h.Silent()

	if asked := h.askedFor(); len(asked) != 1 || asked[0] != "oHg5SJYRHA0" {
		t.Errorf("oembed was asked about %v, want the one id", asked)
	}
}

func TestCooldownHoldsTheChannel(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "#chan", "https://youtu.be/"+testID)
	h.Next()

	// A different video, still inside the channel's cooldown.
	h.knows("oHg5SJYRHA0", video{Title: "Another one", Author: "Someone"})
	h.Say("someone", "#chan", "https://youtu.be/oHg5SJYRHA0")
	h.Silent()

	h.advance(11 * time.Second)
	h.Say("someone", "#chan", "https://youtu.be/oHg5SJYRHA0")
	if got := h.Next(); !strings.Contains(got, "Another one") {
		t.Errorf("bot said %q, want the second video once the cooldown passed", got)
	}
}

func TestDoesNotRepeatTheSameVideo(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "#chan", "https://youtu.be/"+testID)
	h.Next()

	// Past the cooldown, but well inside the repeat window.
	h.advance(time.Minute)
	h.Say("friend", "#chan", "https://www.youtube.com/watch?v="+testID)
	h.Silent()

	h.advance(10 * time.Minute)
	h.Say("friend", "#chan", "https://www.youtube.com/watch?v="+testID)
	if got := h.Next(); !strings.Contains(got, "Never Gonna Give You Up") {
		t.Errorf("bot said %q, want the video again once the repeat window passed", got)
	}
}

func TestNamesEveryVideoInOneLineUpToMax(t *testing.T) {
	h := newHarness(t)
	h.knows("oHg5SJYRHA0", video{Title: "Another one", Author: "Someone"})
	h.knows("QH2-TGUlwu4", video{Title: "A third", Author: "Someone"})

	h.Say("someone", "#chan", "https://youtu.be/"+testID+
		" https://youtu.be/oHg5SJYRHA0 https://youtu.be/QH2-TGUlwu4")

	if got := h.Next(); !strings.Contains(got, "Never Gonna Give You Up") {
		t.Errorf("first line was %q", got)
	}
	if got := h.Next(); !strings.Contains(got, "Another one") {
		t.Errorf("second line was %q", got)
	}
	h.Silent()
}

func TestIgnoredChannelStaysQuiet(t *testing.T) {
	h := newHarness(t)

	h.Say("someone", "#quiet", "https://youtu.be/"+testID)
	h.Silent()
}

func TestIgnoredNickGetsNoPreview(t *testing.T) {
	h := newHarness(t)

	h.Say("spammer", "#chan", "https://youtu.be/"+testID)
	h.Silent()
}

func TestCommandsDoNotReachTheWatcher(t *testing.T) {
	h := newHarness(t)

	// !code is a real command, and it answers with its own line.
	h.Say("someone", "#chan", "!code https://youtu.be/"+testID)
	if got := h.Next(); strings.Contains(got, "YouTube:") {
		t.Errorf("bot previewed a link inside a command: %q", got)
	}
	h.Silent()
}

func TestOembedErrorSaysNothing(t *testing.T) {
	h := newHarness(t)
	h.fails(testID, http.StatusInternalServerError)

	h.Say("someone", "#chan", "https://youtu.be/"+testID)
	h.Silent()
}

func TestLineIsCleanedAndFitted(t *testing.T) {
	v := video{Title: "a\r\nPRIVMSG #chan :fake", Author: "  some\tone  "}
	if got, want := line("#chan", v), "YouTube: a PRIVMSG #chan :fake (some one)"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}

	long := video{Title: strings.Repeat("\u304b", 400)}
	got := line("#chan", long)
	if n := len("PRIVMSG #chan :" + got + "\r\n"); n > irctext.LineLimit {
		t.Errorf("line is %d bytes, over the %d limit", n, irctext.LineLimit)
	}
}
