// Command chat is a REPL that lets you ask natural-language questions about
// the equipment graph. Each question is sent to an LLM to produce a query —
// Cypher for Neo4j/Memgraph, SPARQL for Ontotext GraphDB — which is then run
// against the graph database; both the query and its results are printed.
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
	"github.com/seblkma/graph-database/internal/ingest"
	"github.com/seblkma/graph-database/internal/llm"
	"github.com/seblkma/graph-database/internal/sparql"
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

// sparqlSchemaDescription describes the RDF shape written by
// internal/ingest's RDF ingestion (see rdf.go).
var sparqlSchemaDescription = fmt.Sprintf(`Prefixes:
  PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
  PREFIX eq: <%s>
Entity IRIs live under <%s> and embed the slash-delimited path id.

Classes (rdf:type):
  eq:Station, eq:Switchboard, eq:Panel, eq:Sensor, eq:Inspection
  Panel entities also have rdf:type eq:Equipment.

Object properties:
  station eq:has switchboard . switchboard eq:has panel . panel eq:has sensor .
  inspection eq:inspects panel . inspection eq:via sensor .

Datatype properties (all in the eq: namespace):
  every entity: eq:id (the slash-delimited path, e.g. "STN/TESTSTATION-3/SWB/TESTBOARD-1/PNL/TESTPANEL-5")
  Station/Switchboard/Panel/Sensor: eq:name (the last path segment)
  Panel: unique_id, asset_id, brand, model, yom (integer), voltage (integer), contract,
         serial_no, recommendation_status, dismiss (boolean), recommended_on, inspected_on,
         created_on, modified_on, c_year (integer), display_color
  Inspection: unique_id, age_at_correction (integer), date_of_reporting, date_of_averting,
              qmax_value (double), rep_rate (double), created_on, modified_on, brand_model,
              c_year (integer), yom (integer), contract
A property is absent when its CSV cell was empty. Dates are plain string
literals like "2024-04-09 02:46:35.559+00".`, ingest.SchemaBase, ingest.EntityBase)

func main() {
	dbName := flag.String("db", "", "graph database backend: neo4j, memgraph or graphdb (default $GRAPH_DB, or neo4j)")
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

	client := llm.New(apiKey, model)
	client.Dialect = backend.Dialect

	var ask func(question string)
	if backend.Name == "graphdb" {
		store := sparql.New(backend.URI, backend.Repository, backend.User, backend.Password)
		if _, err := store.Size(ctx); err != nil {
			log.Fatalf("connect to graphdb (run ingest first?): %v", err)
		}
		ask = func(question string) {
			query, err := client.SPARQLForQuestion(ctx, sparqlSchemaDescription, question)
			if err != nil {
				fmt.Printf("error generating query: %v\n", err)
				return
			}
			fmt.Printf("SPARQL: %s\n", query)

			result, err := store.Select(ctx, query)
			if err != nil {
				fmt.Printf("error running query: %v\n", err)
				return
			}
			printSPARQL(result)
		}
	} else {
		db, err := graphdb.Connect(ctx, backend.URI, backend.User, backend.Password)
		if err != nil {
			log.Fatalf("connect to %s: %v", backend.Name, err)
		}
		defer db.Close(ctx)

		ask = func(question string) {
			cypher, err := client.CypherForQuestion(ctx, schemaDescription, question)
			if err != nil {
				fmt.Printf("error generating query: %v\n", err)
				return
			}
			fmt.Printf("Cypher: %s\n", cypher)

			result, err := db.Run(ctx, cypher, nil)
			if err != nil {
				fmt.Printf("error running query: %v\n", err)
				return
			}
			if len(result.Records) == 0 {
				fmt.Println("(no results)")
				return
			}
			for _, record := range result.Records {
				fmt.Println(formatRecord(record))
			}
		}
	}

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
		ask(question)
	}
}

func printSPARQL(result *sparql.Result) {
	if result.Boolean != nil {
		fmt.Println(*result.Boolean)
		return
	}
	if len(result.Rows) == 0 {
		fmt.Println("(no results)")
		return
	}
	for _, row := range result.Rows {
		parts := make([]string, 0, len(result.Vars))
		for _, v := range result.Vars {
			val, ok := row[v]
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%s", v, formatSPARQLValue(val)))
		}
		fmt.Println(strings.Join(parts, "  "))
	}
}

// formatSPARQLValue shortens entity and schema IRIs back to readable form.
func formatSPARQLValue(v sparql.Value) string {
	if v.Type == "uri" {
		if rest, ok := strings.CutPrefix(v.Value, ingest.EntityBase); ok {
			return rest
		}
		if rest, ok := strings.CutPrefix(v.Value, ingest.SchemaBase); ok {
			return "eq:" + rest
		}
		return "<" + v.Value + ">"
	}
	return v.Value
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
