package deerkins

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

const apiBase = "https://api.cloudflare.com/client/v4"

const maxResponse = 1 << 20

var errNoDeer = errors.New("deerkins: no such deer")

type d1 struct {
	http     *http.Client
	endpoint string
	token    string
}

func newD1(base, account, database, token string, timeout time.Duration) *d1 {
	return &d1{
		http:  &http.Client{Timeout: timeout},
		token: token,
		endpoint: fmt.Sprintf("%s/accounts/%s/d1/database/%s/query",
			strings.TrimSuffix(base, "/"), url.PathEscape(account), url.PathEscape(database)),
	}
}

// row is one piece of art
type row struct {
	Deer     string `json:"deer"`
	Creator  string `json:"creator"`
	Date     string `json:"date"`
	Kinskode string `json:"kinskode"`
}

func (c *d1) byName(ctx context.Context, name string) (*row, error) {
	return c.one(ctx, "SELECT deer, creator, date, kinskode FROM deer WHERE deer = ?1 LIMIT 1", name)
}

func (c *d1) random(ctx context.Context) (*row, error) {
	return c.one(ctx, "SELECT deer, creator, date, kinskode FROM deer ORDER BY RANDOM() LIMIT 1")
}

func (c *d1) latest(ctx context.Context) (*row, error) {
	return c.one(ctx, "SELECT deer, creator, date, kinskode FROM deer ORDER BY date DESC, id DESC LIMIT 1")
}

func (c *d1) count(ctx context.Context) (int, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := c.query(ctx, "SELECT COUNT(*) AS n FROM deer", nil, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].N, nil
}

func (c *d1) one(ctx context.Context, sql string, params ...string) (*row, error) {
	var rows []row
	if err := c.query(ctx, sql, params, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errNoDeer
	}
	return &rows[0], nil
}

func (c *d1) query(ctx context.Context, sql string, params []string, dest any) error {
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
		return fmt.Errorf("deerkins: d1 request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("deerkins: reading d1 response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deerkins: d1 returned %s", resp.Status)
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
		return fmt.Errorf("deerkins: unreadable d1 response: %w", err)
	}
	if !envelope.Success {
		if len(envelope.Errors) > 0 {
			return fmt.Errorf("deerkins: d1 error %d: %s", envelope.Errors[0].Code, envelope.Errors[0].Message)
		}
		return errors.New("deerkins: d1 rejected the query")
	}
	if len(envelope.Result) == 0 || len(envelope.Result[0].Results) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result[0].Results, dest)
}
