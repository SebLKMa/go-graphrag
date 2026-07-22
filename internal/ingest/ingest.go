// Package ingest loads the datapipe CSVs into Neo4j as a graph:
//
//	(:Station)-[:HAS]->(:Switchboard)-[:HAS]->(:Panel:Equipment)-[:HAS]->(:Sensor)
//	(:Inspection)-[:INSPECTS]->(:Panel:Equipment)
//	(:Inspection)-[:VIA]->(:Sensor)
//
// The hierarchy comes from parsing the slash-delimited equipment_id (and,
// for inspections, source_path) columns with the hierarchy package.
package ingest

import (
	"context"
	"fmt"

	"github.com/seblkma/graph-database/internal/graphdb"
	"github.com/seblkma/graph-database/internal/hierarchy"
)

// IngestEquipments loads equipments.csv rows: it builds each row's
// Station/Switchboard/Panel chain and attaches the row's properties (plus an
// :Equipment label) to the leaf Panel node.
func IngestEquipments(ctx context.Context, db *graphdb.DB, rows []map[string]string) error {
	for _, row := range rows {
		path := row["equipment_id"]
		nodes := hierarchy.Parse(path)
		if len(nodes) == 0 {
			continue
		}
		if err := mergeChain(ctx, db, nodes); err != nil {
			return fmt.Errorf("equipment %s: %w", path, err)
		}

		leaf := nodes[len(nodes)-1]
		cypher := fmt.Sprintf(`MATCH (n:%s {id: $id}) SET n:Equipment SET n += $props`, leaf.Label)
		if _, err := db.Run(ctx, cypher, map[string]any{
			"id":    leaf.ID,
			"props": equipmentProps(row),
		}); err != nil {
			return fmt.Errorf("equipment %s: set properties: %w", path, err)
		}
	}
	return nil
}

// IngestInspections loads equipment_health_inspections.csv rows: it ensures
// the inspected equipment's hierarchy exists (via equipment_id), optionally
// extends it down to a Sensor (via source_path), and attaches an Inspection
// node connected to both.
func IngestInspections(ctx context.Context, db *graphdb.DB, rows []map[string]string) error {
	for _, row := range rows {
		equipmentID := row["equipment_id"]
		inspectionID := row["inspection_id"]
		if equipmentID == "" || inspectionID == "" {
			continue
		}

		eqNodes := hierarchy.Parse(equipmentID)
		if len(eqNodes) == 0 {
			continue
		}
		if err := mergeChain(ctx, db, eqNodes); err != nil {
			return fmt.Errorf("inspection %s: %w", inspectionID, err)
		}
		panel := eqNodes[len(eqNodes)-1]
		markEquipment := fmt.Sprintf(`MATCH (n:%s {id: $id}) SET n:Equipment`, panel.Label)
		if _, err := db.Run(ctx, markEquipment, map[string]any{"id": panel.ID}); err != nil {
			return fmt.Errorf("inspection %s: %w", inspectionID, err)
		}

		var sensorID string
		if sourcePath := row["source_path"]; sourcePath != "" {
			pathNodes := hierarchy.Parse(sourcePath)
			if err := mergeChain(ctx, db, pathNodes); err != nil {
				return fmt.Errorf("inspection %s: sensor path: %w", inspectionID, err)
			}
			if n := len(pathNodes); n > 0 && pathNodes[n-1].Label == "Sensor" {
				sensorID = pathNodes[n-1].ID
			}
		}

		params := map[string]any{
			"inspectionID": inspectionID,
			"panelID":      panel.ID,
			"props":        inspectionProps(row),
		}
		cypher := `MATCH (panel {id: $panelID})
MERGE (i:Inspection {id: $inspectionID})
SET i += $props
MERGE (i)-[:INSPECTS]->(panel)`
		if sensorID != "" {
			params["sensorID"] = sensorID
			cypher = `MATCH (panel {id: $panelID})
MATCH (sensor:Sensor {id: $sensorID})
MERGE (i:Inspection {id: $inspectionID})
SET i += $props
MERGE (i)-[:INSPECTS]->(panel)
MERGE (i)-[:VIA]->(sensor)`
		}
		if _, err := db.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("inspection %s: %w", inspectionID, err)
		}
	}
	return nil
}

// mergeChain MERGEs each node in a hierarchy.Parse chain and a :HAS
// relationship from each node to the next. It's idempotent, so re-running it
// for a chain (or a prefix of one) already in the graph is a no-op.
func mergeChain(ctx context.Context, db *graphdb.DB, nodes []hierarchy.Node) error {
	var parentID string
	for i, n := range nodes {
		if i == 0 {
			cypher := fmt.Sprintf(`MERGE (n:%s {id: $id}) ON CREATE SET n.name = $name`, n.Label)
			if _, err := db.Run(ctx, cypher, map[string]any{"id": n.ID, "name": n.Name}); err != nil {
				return err
			}
		} else {
			cypher := fmt.Sprintf(`MATCH (p {id: $parentID})
MERGE (n:%s {id: $id})
ON CREATE SET n.name = $name
MERGE (p)-[:HAS]->(n)`, n.Label)
			if _, err := db.Run(ctx, cypher, map[string]any{
				"parentID": parentID,
				"id":       n.ID,
				"name":     n.Name,
			}); err != nil {
				return err
			}
		}
		parentID = n.ID
	}
	return nil
}
