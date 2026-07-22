// Command chat is a REPL that lets you ask natural-language questions about
// the equipment graph. Each question is sent to an LLM to produce a Cypher
// query, which is then run against Neo4j; both the query and its results are
// printed.
package main

import (
	"bufio"
	"context"
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
	ctx := context.Background()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY must be set")
	}
	model := config.EnvOr("ANTHROPIC_MODEL", "claude-sonnet-5")

	uri := config.EnvOr("NEO4J_URI", "bolt://localhost:7687")
	user := config.EnvOr("NEO4J_USER", "neo4j")
	password := config.EnvOr("NEO4J_PASSWORD", "graph4fun")

	db, err := graphdb.Connect(ctx, uri, user, password)
	if err != nil {
		log.Fatalf("connect to neo4j: %v", err)
	}
	defer db.Close(ctx)

	client := llm.New(apiKey, model)

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
