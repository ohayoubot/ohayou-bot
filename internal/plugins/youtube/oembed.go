package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// oembedBase is youtube's public oembed endpoint
const oembedBase = "https://www.youtube.com/oembed"

// maxResponse bounds what is read from a reply, matching the d1 client.
const maxResponse = 1 << 20

// errNoVideo is a video youtube will not describe: deleted, private, or an id
// that was never real
var errNoVideo = errors.New("no such video")

type client struct {
	http *http.Client
	base string
}

func newClient(base string, timeout time.Duration) *client {
	return &client{http: &http.Client{Timeout: timeout}, base: base}
}

// video is the part of an oembed document we care about
type video struct {
	Title  string `json:"title"`
	Author string `json:"author_name"`
}

// lookup asks youtube to describe a video id.
func (c *client) lookup(ctx context.Context, id string) (video, error) {
	// The canonical watch url is what gets asked about whatever the link in
	// the channel looked like, so shorts and youtu.be need no special case.
	q := url.Values{
		"url":    {"https://www.youtube.com/watch?v=" + id},
		"format": {"json"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return video{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return video{}, fmt.Errorf("oembed request: %w", err)
	}
	defer resp.Body.Close()

	switch {
	// An id shaped right but belonging to nothing is a 400, a private video a
	// 401, a deleted one a 404. All three are ordinary answers about a link
	// somebody mistyped or a video that has since gone, so none of them is
	// worth a line in the log.
	case resp.StatusCode == http.StatusBadRequest,
		resp.StatusCode == http.StatusNotFound,
		resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode == http.StatusForbidden:
		return video{}, errNoVideo
	case resp.StatusCode != http.StatusOK:
		return video{}, fmt.Errorf("oembed: %s", resp.Status)
	}

	var v video
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponse)).Decode(&v); err != nil {
		return video{}, fmt.Errorf("oembed decode: %w", err)
	}
	if v.Title == "" {
		return video{}, errNoVideo
	}
	return v, nil
}
