// Command ingest loads datapipe/equipments.csv and
// datapipe/equipment_health_inspections.csv into Neo4j as a graph.
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
	uri := flag.String("uri", config.EnvOr("NEO4J_URI", "bolt://localhost:7687"), "Neo4j bolt URI")
	user := flag.String("user", config.EnvOr("NEO4J_USER", "neo4j"), "Neo4j username")
	password := flag.String("password", config.EnvOr("NEO4J_PASSWORD", "graph4fun"), "Neo4j password")
	equipmentsPath := flag.String("equipments", "datapipe/equipments.csv", "path to equipments CSV")
	inspectionsPath := flag.String("inspections", "datapipe/equipment_health_inspections.csv", "path to inspections CSV")
	flag.Parse()

	ctx := context.Background()

	db, err := graphdb.Connect(ctx, *uri, *user, *password)
	if err != nil {
		log.Fatalf("connect to neo4j: %v", err)
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
