// Package graphdb is a thin wrapper around the Neo4j Bolt driver used by both
// the ingestion and chat commands. It works against any Bolt-speaking graph
// database: Neo4j and Memgraph are the two supported backends.
package graphdb

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// DB holds a connected Bolt driver.
type DB struct {
	driver neo4j.DriverWithContext
}

// Connect opens a driver against uri and verifies connectivity. Empty user
// and password connect without authentication (Memgraph's default setup).
func Connect(ctx context.Context, uri, user, password string) (*DB, error) {
	auth := neo4j.BasicAuth(user, password, "")
	if user == "" && password == "" {
		auth = neo4j.NoAuth()
	}
	driver, err := neo4j.NewDriverWithContext(uri, auth)
	if err != nil {
		return nil, fmt.Errorf("create graph driver: %w", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("verify graph database connectivity at %s: %w", uri, err)
	}
	return &DB{driver: driver}, nil
}

// Close releases the underlying driver.
func (db *DB) Close(ctx context.Context) error {
	return db.driver.Close(ctx)
}

// Run executes a single Cypher statement and returns all of its results.
func (db *DB) Run(ctx context.Context, cypher string, params map[string]any) (*neo4j.EagerResult, error) {
	return neo4j.ExecuteQuery(ctx, db.driver, cypher, params, neo4j.EagerResultTransformer)
}
