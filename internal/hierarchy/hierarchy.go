// Package hierarchy parses the slash-delimited equipment paths used
// throughout the datapipe CSVs (e.g. "STN/TESTSTATION-3/SWB/TESTBOARD-1/PNL/TESTPANEL-5")
// into a chain of typed graph nodes.
package hierarchy

import "strings"

// typeLabels maps the type codes used in equipment_id / source_path segments
// to the node label they become in the graph.
var typeLabels = map[string]string{
	"STN": "Station",
	"SWB": "Switchboard",
	"PNL": "Panel",
	"SSR": "Sensor",
}

// Node is one level of a parsed hierarchy path.
type Node struct {
	Label string // graph label, e.g. "Station"
	Name  string // the path segment's name, e.g. "TESTSTATION-3"
	ID    string // the path up to and including this segment, used as the node's unique id
}

// Parse walks a slash-delimited path in (type, name) pairs and returns one
// Node per pair recognized in typeLabels. It stops at the first segment
// whose type code is unknown, which lets it also be used on inspection
// source_path values that append sensor and date/timestamp segments after
// the equipment path.
func Parse(path string) []Node {
	parts := strings.Split(path, "/")
	var nodes []Node
	var idParts []string
	for i := 0; i+1 < len(parts); i += 2 {
		typeCode, name := parts[i], parts[i+1]
		label, ok := typeLabels[typeCode]
		if !ok {
			break
		}
		idParts = append(idParts, typeCode, name)
		nodes = append(nodes, Node{
			Label: label,
			Name:  name,
			ID:    strings.Join(idParts, "/"),
		})
	}
	return nodes
}
