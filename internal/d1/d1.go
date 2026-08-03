// Package d1 is a small client for the cloudflare D1 http api.
package d1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIBase is cloudflare's api root. Tests point a Client at their own server.
const APIBase = "https://api.cloudflare.com/client/v4"

const maxResponse = 1 << 20

type Client struct {
	http     *http.Client
	endpoint string
	token    string
}

func New(base, account, database, token string, timeout time.Duration) *Client {
	return &Client{
		http:  &http.Client{Timeout: timeout},
		token: token,
		endpoint: fmt.Sprintf("%s/accounts/%s/d1/database/%s/query",
			strings.TrimSuffix(base, "/"), url.PathEscape(account), url.PathEscape(database)),
	}
}

// Query runs sql and unmarshals the first statement's rows into dest.
func (c *Client) Query(ctx context.Context, sql string, params []string, dest any) error {
	if params == nil {
		params = []string{}
	}
	body, err := json.Marshal(struct {
		SQL    string   `json:"sql"`
		Params []string `json:"params"`
	}{sql, params})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("d1 request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("reading d1 response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("d1 returned %s", resp.Status)
	}

	var envelope struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result []struct {
			Results json.RawMessage `json:"results"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("unreadable d1 response: %w", err)
	}
	if !envelope.Success {
		if len(envelope.Errors) > 0 {
			return fmt.Errorf("d1 error %d: %s", envelope.Errors[0].Code, envelope.Errors[0].Message)
		}
		return errors.New("d1 rejected the query")
	}
	if len(envelope.Result) == 0 || len(envelope.Result[0].Results) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result[0].Results, dest)
}
