package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type capture struct {
	path      string
	signature string
	body      []byte
	decoded   body
}

// site stands in for the worker: it records what arrived and answers with
// whatever the test planted.
func site(t *testing.T, answer string, status int) (*httptest.Server, *capture) {
	t.Helper()
	got := &capture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.signature = r.Header.Get("X-Ingest-Signature")
		got.body, _ = io.ReadAll(r.Body)
		_ = json.Unmarshal(got.body, &got.decoded)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func feed(t *testing.T, url string) *Feed {
	t.Helper()
	p := NewPublisher(url, testSecret)
	if p == nil {
		t.Fatal("NewPublisher refused a configured site")
	}
	p.Now = func() time.Time { return time.Unix(testExpiry, 0) }
	return p.For("ohayou")
}

type row struct {
	Account string `json:"account"`
	Acres   int    `json:"acres"`
}

func TestPublishSendsASignedBody(t *testing.T) {
	server, got := site(t, `{"status":"published","rows":1}`, http.StatusOK)

	result, err := feed(t, server.URL).Publish(context.Background(), "plot",
		[]row{{Account: "Mallow", Acres: 6}})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !result.Published() || result.Rows != 1 {
		t.Errorf("result = %+v", result)
	}

	if got.path != "/"+ingestPath {
		t.Errorf("posted to %q, want /%s", got.path, ingestPath)
	}
	if got.decoded.Plugin != "ohayou" || got.decoded.Table != "plot" {
		t.Errorf("body named %s.%s", got.decoded.Plugin, got.decoded.Table)
	}
	if got.decoded.Generation != time.Unix(testExpiry, 0).UnixMilli() {
		t.Errorf("generation = %d", got.decoded.Generation)
	}
	if got.decoded.TS != testExpiry {
		t.Errorf("ts = %d, want %d", got.decoded.TS, testExpiry)
	}
}

// The site verifies this exact construction, and refuses a body signed without
// the prefix. web/tools/ingest.test.mjs asserts the other half.
func TestPublishSignatureCoversThePrefixedBody(t *testing.T) {
	server, got := site(t, `{"status":"published"}`, http.StatusOK)

	if _, err := feed(t, server.URL).Publish(context.Background(), "plot", []row{}); err != nil {
		t.Fatal(err)
	}

	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(ingestPrefix))
	mac.Write(got.body)
	if want := b64(mac.Sum(nil)[:tagBytes]); got.signature != want {
		t.Errorf("signature = %q, want %q", got.signature, want)
	}

	// Without the prefix it would be a tag over the body alone, which is what
	// domain separation exists to make different.
	plain := hmac.New(sha256.New, []byte(testSecret))
	plain.Write(got.body)
	if got.signature == b64(plain.Sum(nil)[:tagBytes]) {
		t.Error("the signature does not cover the prefix")
	}
}

// A generation the site has already seen means a publish that landed, which a
// retry looks like. Not an error.
func TestPublishReportsStaleWithoutFailing(t *testing.T) {
	server, _ := site(t, `{"status":"stale","generation":7}`, http.StatusOK)

	result, err := feed(t, server.URL).Publish(context.Background(), "plot", []row{})
	if err != nil {
		t.Fatalf("a stale publish was an error: %v", err)
	}
	if result.Published() {
		t.Error("a stale publish reported as published")
	}
}

func TestPublishFailsOnARefusal(t *testing.T) {
	server, _ := site(t, `{"status":"error"}`, http.StatusBadRequest)

	if _, err := feed(t, server.URL).Publish(context.Background(), "plot", []row{}); err == nil {
		t.Error("a refused publish was not an error")
	}
}

func TestPublishFailsOnNonsense(t *testing.T) {
	server, _ := site(t, `not json`, http.StatusOK)

	if _, err := feed(t, server.URL).Publish(context.Background(), "plot", []row{}); err == nil {
		t.Error("an unreadable answer was not an error")
	}
}

// Rising, so the site never mistakes a later publish for one it has seen.
func TestGenerationsRise(t *testing.T) {
	server, got := site(t, `{"status":"published"}`, http.StatusOK)

	p := NewPublisher(server.URL, testSecret)
	at := time.Unix(testExpiry, 0)
	p.Now = func() time.Time { return at }
	f := p.For("ohayou")

	ctx := context.Background()
	if _, err := f.Publish(ctx, "plot", []row{}); err != nil {
		t.Fatal(err)
	}
	first := got.decoded.Generation

	at = at.Add(time.Second)
	if _, err := f.Publish(ctx, "plot", []row{}); err != nil {
		t.Fatal(err)
	}
	if got.decoded.Generation <= first {
		t.Errorf("generation went from %d to %d", first, got.decoded.Generation)
	}
}

// A plugin cannot publish under another's name however it builds a body: the
// name comes from the scoping, not the call.
func TestFeedPublishesUnderItsOwnName(t *testing.T) {
	server, got := site(t, `{"status":"published"}`, http.StatusOK)

	p := NewPublisher(server.URL, testSecret)
	p.Now = func() time.Time { return time.Unix(testExpiry, 0) }

	if _, err := p.For("drop").Publish(context.Background(), "plot", []row{}); err != nil {
		t.Fatal(err)
	}
	if got.decoded.Plugin != "drop" {
		t.Errorf("plugin = %q, want drop", got.decoded.Plugin)
	}
}

// A bot with no site configured keeps its game to itself rather than failing
// somewhere later.
func TestNoPublisherWithoutASite(t *testing.T) {
	for name, p := range map[string]*Publisher{
		"no url":    NewPublisher("", testSecret),
		"no secret": NewPublisher("https://hemera.day/", ""),
	} {
		if p != nil {
			t.Errorf("%s: a publisher was returned", name)
		}
		if p.For("ohayou") != nil {
			t.Errorf("%s: a feed was returned", name)
		}
	}
}

// The zero feed is what a plugin gets when there is no site, so it has to say
// so rather than panic.
func TestNilFeedRefusesPolitely(t *testing.T) {
	var f *Feed
	if _, err := f.Publish(context.Background(), "plot", []row{}); err == nil {
		t.Error("publishing to nothing succeeded")
	}
}

func TestEndpointJoinsTheSiteURL(t *testing.T) {
	for _, base := range []string{
		"https://hemera.day/",
		"https://hemera.day",
		"https://hemera.day/#",
	} {
		p := NewPublisher(base, testSecret)
		if got := p.endpoint(); got != "https://hemera.day/"+ingestPath {
			t.Errorf("endpoint(%q) = %q", base, got)
		}
	}
}

func TestPublishHonoursAContext(t *testing.T) {
	server, _ := site(t, `{"status":"published"}`, http.StatusOK)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := feed(t, server.URL).Publish(ctx, "plot", []row{})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a cancelled context", err)
	}
}
