package web

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// A projection is published by POSTing a signed body to the site, which makes
// the write: the bot holds no database credential, so what may be stored is
// decided in web/lib/site/ingest.js rather than wherever a plugin builds a row.
//
// The signature covers ingestPrefix then the body, which is domain separation
// from the grants: a grant's payload is raw bytes starting with a version byte,
// so neither can be replayed as the other under the shared key.
const ingestPrefix = "ingest.v1\n"

// ingestPath is under the same url a link is minted against.
const ingestPath = "api/ingest"

const publishTimeout = 30 * time.Second

// maxResponse bounds a site answering something unexpected.
const maxResponse = 1 << 16

// Publisher sends projections to the site. Plugins use the scoped Feed below.
type Publisher struct {
	http   *http.Client
	url    string
	secret string
	// Now is the clock the generation comes from, injectable for tests.
	Now func() time.Time
}

// NewPublisher returns nil when there is nowhere to publish to.
func NewPublisher(url, secret string) *Publisher {
	if url == "" || secret == "" {
		return nil
	}
	return &Publisher{
		http:   &http.Client{Timeout: publishTimeout},
		url:    strings.TrimSuffix(url, "#"),
		secret: secret,
		Now:    time.Now,
	}
}

// For scopes a publisher to one plugin, which is the only name it can publish
// under however it builds a body.
func (p *Publisher) For(plugin string) *Feed {
	if p == nil {
		return nil
	}
	return &Feed{publisher: p, plugin: plugin}
}

// Feed is one plugin's way of publishing.
type Feed struct {
	publisher *Publisher
	plugin    string
}

// body is what the site is sent. It matches what ingest.js reads.
type body struct {
	Plugin     string `json:"plugin"`
	Table      string `json:"table"`
	Generation int64  `json:"generation"`
	TS         int64  `json:"ts"`
	Rows       any    `json:"rows"`
	// Mode is "append" or empty for a replace, and Keep goes with it.
	Mode string `json:"mode,omitempty"`
	Keep int    `json:"keep,omitempty"`
}

// Result is what the site did with a publish.
type Result struct {
	// Status is "published" or "stale". Stale means the site had already seen a
	// generation this high, which is what a retry of one that landed looks
	// like. Not a failure.
	Status string `json:"status"`
	// Rows is how many were written, Total how many the table holds now. They
	// differ for an append, where Total is how the caller checks the two ends
	// still agree.
	Rows  int `json:"rows"`
	Total int `json:"total"`
}

// Published reports whether the rows were taken.
func (r Result) Published() bool { return r.Status == "published" }

// Publish replaces one table outright. Not a merge: a player who withdrew
// consent is absent from the next publish, without anything having to remember
// to delete them.
//
// rows must marshal to a json array carrying exactly the columns ingest.js
// allows for the table. Anything else is refused there, so this end cannot
// widen what is public by sending more.
func (f *Feed) Publish(ctx context.Context, table string, rows any) (Result, error) {
	return f.post(ctx, body{Table: table, Rows: rows})
}

// Append adds rows to a table and leaves the site holding the newest keep of
// them, rather than rewriting the whole table to add one. Only the tables
// ingest.js names as appendable take it, and only rows the site does not
// already hold differently: an append cannot correct what is there.
func (f *Feed) Append(ctx context.Context, table string, rows any, keep int) (Result, error) {
	return f.post(ctx, body{Table: table, Rows: rows, Mode: "append", Keep: keep})
}

func (f *Feed) post(ctx context.Context, b body) (Result, error) {
	if f == nil {
		return Result{}, fmt.Errorf("web: no site to publish to")
	}

	b.Plugin = f.plugin
	// Milliseconds rather than a counter, so this survives a restart with
	// nothing written down. The site refuses a generation it has seen, so a
	// clock going backwards costs a skipped publish rather than a wrong one.
	b.Generation = f.publisher.Now().UnixMilli()
	b.TS = f.publisher.Now().Unix()

	table := b.Table
	raw, err := json.Marshal(b)
	if err != nil {
		return Result{}, fmt.Errorf("web: encoding a publish: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.publisher.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ingest-Signature", b64(f.publisher.tag(raw)))

	resp, err := f.publisher.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("web: publishing %s: %w", table, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return Result{}, fmt.Errorf("web: reading the site's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("web: publishing %s: the site said %s", table, resp.Status)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, fmt.Errorf("web: unreadable answer from the site: %w", err)
	}
	return result, nil
}

func (p *Publisher) endpoint() string {
	return strings.TrimSuffix(p.url, "/") + "/" + ingestPath
}

func (p *Publisher) tag(raw []byte) []byte {
	mac := hmac.New(sha256.New, []byte(p.secret))
	mac.Write([]byte(ingestPrefix))
	mac.Write(raw)
	return mac.Sum(nil)[:tagBytes]
}
