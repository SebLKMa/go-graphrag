// RDF ingestion for the Ontotext GraphDB backend. The same CSVs and
// hierarchy parsing as ingest.go, expressed as triples instead of a labeled
// property graph:
//
//	entity rdf:type eq:Station|eq:Switchboard|eq:Panel|eq:Sensor|eq:Inspection
//	panel entities additionally rdf:type eq:Equipment
//	parent eq:has child        (hierarchy)
//	inspection eq:inspects panel
//	inspection eq:via sensor
//	CSV columns become eq:* datatype properties (eq:id, eq:name, eq:brand, ...)
//
// Entity IRIs embed the slash-delimited path ids under EntityBase, so the
// same station/switchboard/panel/sensor referenced from different rows maps
// onto one resource. INSERT DATA of an existing triple is a no-op in RDF, so
// ingestion stays idempotent like its Cypher counterpart.
package ingest

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/seblkma/graph-database/internal/hierarchy"
	"github.com/seblkma/graph-database/internal/sparql"
)

const (
	// SchemaBase is the namespace for classes and predicates (PREFIX eq:).
	SchemaBase = "http://graphrag.example/schema#"
	// EntityBase is the namespace under which entity IRIs live.
	EntityBase = "http://graphrag.example/entity/"

	rdfPrefixes = "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nPREFIX eq: <" + SchemaBase + ">\n"
)

// IngestEquipmentsRDF is IngestEquipments for the GraphDB backend.
func IngestEquipmentsRDF(ctx context.Context, c *sparql.Client, rows []map[string]string) error {
	for _, row := range rows {
		path := row["equipment_id"]
		nodes := hierarchy.Parse(path)
		if len(nodes) == 0 {
			continue
		}
		triples := chainTriples(nodes)

		leaf := entityIRI(nodes[len(nodes)-1].ID)
		triples = append(triples, fmt.Sprintf("%s rdf:type eq:Equipment .", leaf))
		triples = append(triples, propertyTriples(leaf, equipmentProps(row))...)

		if err := insertData(ctx, c, triples); err != nil {
			return fmt.Errorf("equipment %s: %w", path, err)
		}
	}
	return nil
}

// IngestInspectionsRDF is IngestInspections for the GraphDB backend.
func IngestInspectionsRDF(ctx context.Context, c *sparql.Client, rows []map[string]string) error {
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
		triples := chainTriples(eqNodes)
		panel := entityIRI(eqNodes[len(eqNodes)-1].ID)
		triples = append(triples, fmt.Sprintf("%s rdf:type eq:Equipment .", panel))

		var sensor string
		if sourcePath := row["source_path"]; sourcePath != "" {
			pathNodes := hierarchy.Parse(sourcePath)
			triples = append(triples, chainTriples(pathNodes)...)
			if n := len(pathNodes); n > 0 && pathNodes[n-1].Label == "Sensor" {
				sensor = entityIRI(pathNodes[n-1].ID)
			}
		}

		inspection := entityIRI("inspection/" + inspectionID)
		triples = append(triples,
			fmt.Sprintf("%s rdf:type eq:Inspection .", inspection),
			fmt.Sprintf("%s eq:id %s .", inspection, rdfLiteral(inspectionID)),
			fmt.Sprintf("%s eq:inspects %s .", inspection, panel),
		)
		if sensor != "" {
			triples = append(triples, fmt.Sprintf("%s eq:via %s .", inspection, sensor))
		}
		triples = append(triples, propertyTriples(inspection, inspectionProps(row))...)

		if err := insertData(ctx, c, triples); err != nil {
			return fmt.Errorf("inspection %s: %w", inspectionID, err)
		}
	}
	return nil
}

// chainTriples is mergeChain for RDF: type, id and name triples for each
// node in a hierarchy.Parse chain, plus eq:has between consecutive levels.
func chainTriples(nodes []hierarchy.Node) []string {
	var triples []string
	var parent string
	for _, n := range nodes {
		iri := entityIRI(n.ID)
		triples = append(triples,
			fmt.Sprintf("%s rdf:type eq:%s .", iri, n.Label),
			fmt.Sprintf("%s eq:id %s .", iri, rdfLiteral(n.ID)),
			fmt.Sprintf("%s eq:name %s .", iri, rdfLiteral(n.Name)),
		)
		if parent != "" {
			triples = append(triples, fmt.Sprintf("%s eq:has %s .", parent, iri))
		}
		parent = iri
	}
	return triples
}

// propertyTriples turns an equipmentProps/inspectionProps map into eq:<key>
// datatype-property triples on subject, in stable key order.
func propertyTriples(subject string, props map[string]any) []string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	triples := make([]string, 0, len(keys))
	for _, k := range keys {
		triples = append(triples, fmt.Sprintf("%s eq:%s %s .", subject, k, rdfLiteral(props[k])))
	}
	return triples
}

func insertData(ctx context.Context, c *sparql.Client, triples []string) error {
	update := rdfPrefixes + "INSERT DATA {\n  " + strings.Join(triples, "\n  ") + "\n}"
	return c.Update(ctx, update)
}

// entityIRI returns the angle-bracketed IRI for a path-shaped id, keeping
// the slashes readable while percent-escaping anything an IRI can't hold.
func entityIRI(id string) string {
	segments := strings.Split(id, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return "<" + EntityBase + strings.Join(segments, "/") + ">"
}

// rdfLiteral renders a props value as a SPARQL literal.
func rdfLiteral(v any) string {
	switch val := v.(type) {
	case string:
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
		return `"` + r.Replace(val) + `"`
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'E', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}
