// Package sparql is a thin HTTP client for Ontotext GraphDB's RDF4J-style
// REST API: repository management under /rest/repositories, SPARQL updates at
// /repositories/{id}/statements and SPARQL queries at /repositories/{id}.
package sparql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client talks to one GraphDB repository.
type Client struct {
	BaseURL    string // e.g. http://localhost:7200
	Repository string
	User       string
	Password   string
	HTTPClient *http.Client
}

// New returns a Client for the GraphDB instance at baseURL. Empty user and
// password connect without authentication (GraphDB's default setup).
func New(baseURL, repository, user, password string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Repository: repository,
		User:       user,
		Password:   password,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.User != "" || c.Password != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	return c.HTTPClient.Do(req)
}

// EnsureRepository creates the repository if it does not exist yet.
func (c *Client) EnsureRepository(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/rest/repositories/"+c.Repository, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("check repository %q at %s: %w", c.Repository, c.BaseURL, err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return c.createRepository(ctx)
	default:
		return fmt.Errorf("check repository %q: unexpected status %d", c.Repository, resp.StatusCode)
	}
}

func (c *Client) createRepository(ctx context.Context) error {
	body, err := json.Marshal(map[string]any{
		"id":    c.Repository,
		"type":  "graphdb",
		"title": "go-graphrag equipment graph",
	})
	if err != nil {
		return fmt.Errorf("marshal repository config: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/rest/repositories", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("create repository %q: %w", c.Repository, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create repository %q: status %d: %s", c.Repository, resp.StatusCode, msg)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// Size returns the number of statements in the repository. It doubles as a
// connectivity (and repository existence) check.
func (c *Client) Size(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/repositories/"+c.Repository+"/size", nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return 0, fmt.Errorf("reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("repository %q: status %d: %s", c.Repository, resp.StatusCode, body)
	}
	var n int64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(body)), "%d", &n); err != nil {
		return 0, fmt.Errorf("parse repository size %q: %w", body, err)
	}
	return n, nil
}

// Update runs a SPARQL update (e.g. INSERT DATA) against the repository.
func (c *Client) Update(ctx context.Context, update string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/repositories/"+c.Repository+"/statements", strings.NewReader(update))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/sparql-update")
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("run update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed: status %d: %s", resp.StatusCode, msg)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// Value is one bound value in a SPARQL result: an IRI ("uri"), a literal, or
// a blank node, per the SPARQL 1.1 JSON results format.
type Value struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Datatype string `json:"datatype,omitempty"`
	Lang     string `json:"xml:lang,omitempty"`
}

// Result holds the rows of a SELECT query (or the answer to an ASK query).
type Result struct {
	Vars    []string
	Rows    []map[string]Value
	Boolean *bool // set for ASK queries
}

// Select runs a read-only SPARQL query (SELECT or ASK) and returns its rows.
func (c *Client) Select(ctx context.Context, query string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/repositories/"+c.Repository, strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("run query: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query failed: status %d: %s", resp.StatusCode, body)
	}

	var parsed struct {
		Head struct {
			Vars []string `json:"vars"`
		} `json:"head"`
		Results *struct {
			Bindings []map[string]Value `json:"bindings"`
		} `json:"results"`
		Boolean *bool `json:"boolean"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse results: %w (body: %.200s)", err, body)
	}
	result := &Result{Vars: parsed.Head.Vars, Boolean: parsed.Boolean}
	if parsed.Results != nil {
		result.Rows = parsed.Results.Bindings
	}
	return result, nil
}
