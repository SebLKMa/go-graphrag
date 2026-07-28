// Package llm calls the Anthropic Messages API to translate a natural
// language question about the graph into a single Cypher query.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	apiURL     = "https://api.anthropic.com/v1/messages"
	apiVersion = "2023-06-01"
)

// Client calls the Anthropic Messages API.
type Client struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// Dialect names the graph database whose Cypher dialect generated
	// queries should target, e.g. "Neo4j" or "Memgraph". Empty means Neo4j.
	Dialect string
}

// New returns a Client for the given API key and model id.
func New(apiKey, model string) *Client {
	return &Client{APIKey: apiKey, Model: model, HTTPClient: &http.Client{}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiError struct {
	Message string `json:"message"`
}

type response struct {
	Content []contentBlock `json:"content"`
	Error   *apiError      `json:"error"`
}

// CypherForQuestion asks the model to translate question into a single
// read-only Cypher query, given a description of the graph schema.
func (c *Client) CypherForQuestion(ctx context.Context, schema, question string) (string, error) {
	dialect := c.Dialect
	if dialect == "" {
		dialect = "Neo4j"
	}
	system := fmt.Sprintf(`You translate natural-language questions into Cypher queries for a %s graph database. Use only Cypher constructs that %s supports.

Schema:
%s

Rules:
- Respond with exactly one Cypher query and nothing else: no markdown fences, no explanation.
- The query must be read-only (MATCH/RETURN); never modify the graph (no CREATE, MERGE, SET, DELETE).
- Limit results to 25 rows unless the question specifies otherwise.`, dialect, dialect, schema)

	body, err := json.Marshal(request{
		Model:     c.Model,
		MaxTokens: 1024,
		System:    system,
		Messages:  []message{{Role: "user", Content: question}},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call anthropic api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var parsed response
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse response: %w (status %d, body: %s)", err, resp.StatusCode, respBody)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("anthropic api error: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic api returned status %d: %s", resp.StatusCode, respBody)
	}
	if len(parsed.Content) == 0 {
		return "", fmt.Errorf("empty response from anthropic api")
	}

	query := strings.TrimSpace(parsed.Content[0].Text)
	query = strings.TrimPrefix(query, "```cypher")
	query = strings.TrimPrefix(query, "```")
	query = strings.TrimSuffix(query, "```")
	return strings.TrimSpace(query), nil
}
