// Package lokiquery wraps the Loki /loki/api/v1/query_range endpoint with a
// pagination loop. The caller passes a LogQL selector plus a [start, end]
// window in nanoseconds; we walk forward in pages of `limit` entries until
// drained.
package lokiquery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultPageLimit = 5000

type Client struct {
	URL    string
	Tenant string
	HTTP   *http.Client
}

func New(loki, tenant string) *Client {
	return &Client{
		URL:    loki,
		Tenant: tenant,
		HTTP:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Entry is one log line with its labels and timestamp.
type Entry struct {
	Time   time.Time
	Labels map[string]string
	Line   string
}

// QueryRange returns every entry matching `selector` (a LogQL stream selector,
// e.g. `{experiment="foo"}`) in the time window. Entries come back in
// forward chronological order.
func (c *Client) QueryRange(ctx context.Context, selector string, start, end time.Time) ([]Entry, error) {
	var out []Entry
	cursor := start
	for {
		page, err := c.queryPage(ctx, selector, cursor, end, defaultPageLimit)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		if len(page) < defaultPageLimit {
			break
		}
		// Advance cursor by 1 ns past the last entry to avoid re-fetching it.
		cursor = page[len(page)-1].Time.Add(time.Nanosecond)
		if !cursor.Before(end) {
			break
		}
	}
	return out, nil
}

func (c *Client) queryPage(ctx context.Context, selector string, start, end time.Time, limit int) ([]Entry, error) {
	q := url.Values{}
	q.Set("query", selector)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("direction", "forward")

	endpoint := c.URL + "/loki/api/v1/query_range?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.Tenant != "" {
		req.Header.Set("X-Scope-OrgID", c.Tenant)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("loki query: status %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	var out []Entry
	for _, stream := range body.Data.Result {
		for _, kv := range stream.Values {
			ns, err := strconv.ParseInt(kv[0], 10, 64)
			if err != nil {
				continue
			}
			out = append(out, Entry{
				Time:   time.Unix(0, ns),
				Labels: stream.Stream,
				Line:   kv[1],
			})
		}
	}
	// Loki returns streams grouped, not globally sorted by time within a page.
	sortByTime(out)
	return out, nil
}

func sortByTime(es []Entry) {
	// insertion sort is fine — entries inside one stream arrive sorted, and
	// the number of streams per page is small.
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j-1].Time.After(es[j].Time); j-- {
			es[j-1], es[j] = es[j], es[j-1]
		}
	}
}
