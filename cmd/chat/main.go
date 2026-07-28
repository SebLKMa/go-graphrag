// Command chat is a REPL that lets you ask natural-language questions about
// the equipment graph. Each question is sent to an LLM to produce a Cypher
// query, which is then run against the graph database (Neo4j or Memgraph);
// both the query and its results are printed.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/seblkma/graph-database/internal/config"
	"github.com/seblkma/graph-database/internal/graphdb"
	"github.com/seblkma/graph-database/internal/llm"
)

const schemaDescription = `Nodes:
  (:Station {id, name})
  (:Switchboard {id, name})
  (:Panel:Equipment {id, name, unique_id, asset_id, brand, model, yom, voltage, contract,
                      serial_no, recommendation_status, dismiss, recommended_on, inspected_on,
                      created_on, modified_on, c_year, display_color})
  (:Sensor {id, name})
  (:Inspection {id, unique_id, age_at_correction, date_of_reporting, date_of_averting,
                qmax_value, rep_rate, created_on, modified_on, brand_model, c_year, yom, contract})

Relationships:
  (:Station)-[:HAS]->(:Switchboard)
  (:Switchboard)-[:HAS]->(:Panel)
  (:Panel)-[:HAS]->(:Sensor)
  (:Inspection)-[:INSPECTS]->(:Panel)
  (:Inspection)-[:VIA]->(:Sensor)`

func main() {
	dbName := flag.String("db", "", "graph database backend: neo4j or memgraph (default $GRAPH_DB, or neo4j)")
	flag.Parse()

	ctx := context.Background()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY must be set")
	}
	model := config.EnvOr("ANTHROPIC_MODEL", "claude-sonnet-5")

	backend, err := config.GraphBackend(*dbName)
	if err != nil {
		log.Fatal(err)
	}

	db, err := graphdb.Connect(ctx, backend.URI, backend.User, backend.Password)
	if err != nil {
		log.Fatalf("connect to %s: %v", backend.Name, err)
	}
	defer db.Close(ctx)

	client := llm.New(apiKey, model)
	client.Dialect = backend.Dialect

	fmt.Println("Ask questions about the equipment graph. Type 'exit' or 'quit' to leave.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		question := strings.TrimSpace(scanner.Text())
		if question == "" {
			continue
		}
		if question == "exit" || question == "quit" {
			break
		}

		cypher, err := client.CypherForQuestion(ctx, schemaDescription, question)
		if err != nil {
			fmt.Printf("error generating query: %v\n", err)
			continue
		}
		fmt.Printf("Cypher: %s\n", cypher)

		result, err := db.Run(ctx, cypher, nil)
		if err != nil {
			fmt.Printf("error running query: %v\n", err)
			continue
		}
		if len(result.Records) == 0 {
			fmt.Println("(no results)")
			continue
		}
		for _, record := range result.Records {
			fmt.Println(formatRecord(record))
		}
	}
}

func formatRecord(record *neo4j.Record) string {
	parts := make([]string, len(record.Keys))
	for i, key := range record.Keys {
		parts[i] = fmt.Sprintf("%s=%s", key, formatValue(record.Values[i]))
	}
	return strings.Join(parts, "  ")
}

func formatValue(v any) string {
	switch val := v.(type) {
	case neo4j.Node:
		return fmt.Sprintf("(%s %v)", strings.Join(val.Labels, ":"), val.Props)
	case neo4j.Relationship:
		return fmt.Sprintf("[:%s %v]", val.Type, val.Props)
	default:
		return fmt.Sprintf("%v", val)
	}
}
