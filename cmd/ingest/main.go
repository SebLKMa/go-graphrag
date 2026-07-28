// Command ingest loads datapipe/equipments.csv and
// datapipe/equipment_health_inspections.csv into a graph database
// (Neo4j or Memgraph) as a graph.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/seblkma/graph-database/internal/config"
	"github.com/seblkma/graph-database/internal/graphdb"
	"github.com/seblkma/graph-database/internal/ingest"
)

func main() {
	dbName := flag.String("db", "", "graph database backend: neo4j or memgraph (default $GRAPH_DB, or neo4j)")
	uri := flag.String("uri", "", "bolt URI (default per backend)")
	user := flag.String("user", "", "username (default per backend)")
	password := flag.String("password", "", "password (default per backend)")
	equipmentsPath := flag.String("equipments", "datapipe/equipments.csv", "path to equipments CSV")
	inspectionsPath := flag.String("inspections", "datapipe/equipment_health_inspections.csv", "path to inspections CSV")
	flag.Parse()

	backend, err := config.GraphBackend(*dbName)
	if err != nil {
		log.Fatal(err)
	}
	if *uri != "" {
		backend.URI = *uri
	}
	if *user != "" {
		backend.User = *user
	}
	if *password != "" {
		backend.Password = *password
	}

	ctx := context.Background()

	db, err := graphdb.Connect(ctx, backend.URI, backend.User, backend.Password)
	if err != nil {
		log.Fatalf("connect to %s: %v", backend.Name, err)
	}
	defer db.Close(ctx)

	equipmentRows, err := ingest.ReadCSV(*equipmentsPath)
	if err != nil {
		log.Fatalf("read equipments csv: %v", err)
	}
	if err := ingest.IngestEquipments(ctx, db, equipmentRows); err != nil {
		log.Fatalf("ingest equipments: %v", err)
	}
	fmt.Printf("ingested %d equipment rows\n", len(equipmentRows))

	inspectionRows, err := ingest.ReadCSV(*inspectionsPath)
	if err != nil {
		log.Fatalf("read inspections csv: %v", err)
	}
	if err := ingest.IngestInspections(ctx, db, inspectionRows); err != nil {
		log.Fatalf("ingest inspections: %v", err)
	}
	fmt.Printf("ingested %d inspection rows\n", len(inspectionRows))
}
