package ingest

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
)

// ReadCSV reads path into a slice of rows, each keyed by the header column name.
func ReadCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header of %s: %w", path, err)
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row of %s: %w", path, err)
		}
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				row[col] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
