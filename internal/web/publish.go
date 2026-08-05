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
// the write. The bot holds no database credential of its own: what may be
// stored is decided in web/lib/site/ingest.js, in one place, rather than
// wherever a plugin happens to build a row.
//
// The signature covers ingestPrefix followed by the body's bytes. That prefix
// is domain separation: a grant's payload is raw bytes beginning with a version
// byte, so neither can ever be replayed as the other even though the key is the
// same one.
const ingestPrefix = "ingest.v1\n"

// ingestPath is where the site listens, under the same url a link is minted
// against.
const ingestPath = "api/ingest"

const publishTimeout = 30 * time.Second

// maxResponse bounds what is read back from a site answering something unexpected.
const maxResponse = 1 << 16

// Publisher sends projections to the site. Plugins use the scoped Feed below.
type Publisher struct {
	http   *http.Client
	url    string
	secret string
	// Now is the clock the generation comes from, injectable for tests.
	Now func() time.Time
}

// NewPublisher returns a publisher, or nil when there is nowhere to publish to:
// a bot with no site configured keeps its game to itself.
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
}

// Result is what the site did with a publish.
type Result struct {
	// Status is "published" or "stale". Stale means the site had already seen a
	// generation at least this high, which a retry of a publish that landed
	// looks like. It is not a failure.
	Status string `json:"status"`
	Rows   int    `json:"rows"`
}

// Published reports whether the rows were taken.
func (r Result) Published() bool { return r.Status == "published" }

// Publish replaces the whole of one table with rows. It is not a merge: a
// player who withdrew consent is absent from the next publish and so from the
// site, without anything having to remember to delete them.
//
// rows must marshal to a json array whose objects carry exactly the columns
// ingest.js allows for the table. Anything else is refused there, which is the
// point: this end cannot widen what is public by sending more.
func (f *Feed) Publish(ctx context.Context, table string, rows any) (Result, error) {
	if f == nil {
		return Result{}, fmt.Errorf("web: no site to publish to")
	}

	// Milliseconds rather than a counter, so a generation survives a restart
	// without anything having to be written down. The site refuses one it has
	// already seen, so a clock that goes backwards costs a skipped publish
	// rather than a wrong one.
	generation := f.publisher.Now().UnixMilli()

	raw, err := json.Marshal(body{
		Plugin:     f.plugin,
		Table:      table,
		Generation: generation,
		TS:         f.publisher.Now().Unix(),
		Rows:       rows,
	})
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
		// The site answers every refusal the same way and logs the reason, so
		// there is nothing here worth parsing out.
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
