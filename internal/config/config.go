// Package config holds small helpers shared by the ingest and chat commands.
package config

import (
	"fmt"
	"os"
	"strings"
)

// EnvOr returns the environment variable named key, or fallback if it is unset or empty.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Backend describes a graph database to connect to.
type Backend struct {
	Name     string // backend key: "neo4j" or "memgraph"
	Dialect  string // display name for the Cypher dialect, e.g. "Neo4j"
	URI      string
	User     string
	Password string
}

var backends = map[string]Backend{
	"neo4j":    {Name: "neo4j", Dialect: "Neo4j", URI: "bolt://localhost:7687", User: "neo4j", Password: "graph4fun"},
	"memgraph": {Name: "memgraph", Dialect: "Memgraph", URI: "bolt://localhost:7687"}, // no auth by default
}

// GraphBackend resolves which graph database to use. name selects the backend
// ("neo4j" or "memgraph"); when empty, the GRAPH_DB env var decides,
// defaulting to neo4j. Connection details can be overridden with GRAPH_URI,
// GRAPH_USER and GRAPH_PASSWORD (set-but-empty counts, so Memgraph's empty
// credentials are expressible); the legacy NEO4J_URI/NEO4J_USER/NEO4J_PASSWORD
// variables are honored as a fallback.
func GraphBackend(name string) (Backend, error) {
	if name == "" {
		name = EnvOr("GRAPH_DB", "neo4j")
	}
	b, ok := backends[strings.ToLower(name)]
	if !ok {
		return Backend{}, fmt.Errorf("unknown graph database %q (supported: neo4j, memgraph)", name)
	}
	b.URI = envLookupOr(b.URI, "GRAPH_URI", "NEO4J_URI")
	b.User = envLookupOr(b.User, "GRAPH_USER", "NEO4J_USER")
	b.Password = envLookupOr(b.Password, "GRAPH_PASSWORD", "NEO4J_PASSWORD")
	return b, nil
}

// envLookupOr returns the value of the first environment variable in keys
// that is set — even if set to the empty string — or fallback if none are.
func envLookupOr(fallback string, keys ...string) string {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
	}
	return fallback
}
