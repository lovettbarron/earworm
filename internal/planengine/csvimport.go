package planengine

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lovettbarron/earworm/internal/db"
)

// columnAliases maps common real-world header variants to canonical names.
// Keys are lowercase, space/hyphen-normalized.
var columnAliases = map[string]string{
	"op_type":     "op_type",
	"type":        "op_type",
	"operation":   "op_type",
	"action":      "op_type",
	"source_path": "source_path",
	"source":      "source_path",
	"path":        "source_path",
	"src":          "source_path",
	"current_path": "source_path",
	"dest_path":   "dest_path",
	"destination": "dest_path",
	"dest":        "dest_path",
	"target":      "dest_path",
	"title":       "title",
	"author":      "author",
	"narrator":    "narrator",
	"genre":       "genre",
	"year":        "year",
	"series":      "series",
	"asin":        "asin",
}

// metadataColumns lists CSV columns that map to BookMetadata fields.
var metadataColumns = []string{"title", "author", "narrator", "genre", "year", "series", "asin"}

// normalizeHeader converts a raw CSV header to its canonical form.
func normalizeHeader(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	if canonical, ok := columnAliases[s]; ok {
		return canonical
	}
	return s
}

// extractMetadataJSON builds a JSON string from metadata columns in a CSV row.
// Returns empty string if no metadata columns have values.
func extractMetadataJSON(colIndex map[string]int, record []string) string {
	m := make(map[string]interface{})
	for _, col := range metadataColumns {
		idx, ok := colIndex[col]
		if !ok || idx >= len(record) {
			continue
		}
		val := strings.TrimSpace(record[idx])
		if val == "" {
			continue
		}
		if col == "year" {
			if y, err := strconv.Atoi(val); err == nil {
				m["year"] = y
			}
			continue
		}
		m[col] = val
	}
	if len(m) == 0 {
		return ""
	}
	data, _ := json.Marshal(m)
	return string(data)
}

// CSVRowError records a validation error for a specific CSV row.
type CSVRowError struct {
	Line    int
	Column  string
	Message string
}

// CSVImportResult summarises the outcome of a CSV import operation.
type CSVImportResult struct {
	PlanID     int64
	RowCount   int
	ErrorCount int
	Errors     []CSVRowError
}

// StripBOM returns a reader that skips a leading UTF-8 BOM (0xEF 0xBB 0xBF) if present.
func StripBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	peeked, err := br.Peek(3)
	if err == nil && len(peeked) == 3 &&
		peeked[0] == 0xEF && peeked[1] == 0xBB && peeked[2] == 0xBF {
		// Discard the BOM bytes
		_, _ = br.Discard(3)
	}
	return br
}

// ImportCSV parses a CSV reader and creates a draft plan with one operation per valid row.
// If any validation errors are found, no plan is created and errors are returned in the result.
func ImportCSV(database *sql.DB, planName string, r io.Reader) (*CSVImportResult, error) {
	r = StripBOM(r)

	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true

	// Read header row
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	// Build alias-normalized column index map with ambiguity detection
	colIndex := make(map[string]int, len(header))
	seen := make(map[string]string) // canonical -> original raw header
	for i, h := range header {
		canonical := normalizeHeader(h)
		if prev, exists := seen[canonical]; exists {
			return nil, fmt.Errorf("ambiguous columns: %q and %q both map to %q", prev, strings.TrimSpace(h), canonical)
		}
		seen[canonical] = strings.TrimSpace(h)
		colIndex[canonical] = i
	}

	// Validate required columns
	for _, req := range []string{"op_type", "source_path"} {
		if _, ok := colIndex[req]; !ok {
			return nil, fmt.Errorf("missing required column: %s", req)
		}
	}

	// dest_path column index (-1 means not present)
	destIdx := -1
	if idx, ok := colIndex["dest_path"]; ok {
		destIdx = idx
	}

	opTypeIdx := colIndex["op_type"]
	srcIdx := colIndex["source_path"]

	// Read all data rows and validate
	type rowData struct {
		opType   string
		source   string
		dest     string
		metadata string
	}
	var rows []rowData
	var csvErrors []CSVRowError
	lineNum := 1 // header is line 1

	for {
		lineNum++
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row at line %d: %w", lineNum, err)
		}

		opType := strings.TrimSpace(record[opTypeIdx])
		source := strings.TrimSpace(record[srcIdx])
		dest := ""
		if destIdx >= 0 && destIdx < len(record) {
			dest = strings.TrimSpace(record[destIdx])
		}

		// Validate op_type
		if !db.IsValidOpType(opType) {
			csvErrors = append(csvErrors, CSVRowError{
				Line:    lineNum,
				Column:  "op_type",
				Message: fmt.Sprintf("invalid operation type %q", opType),
			})
		}

		// Validate source_path non-empty
		if source == "" {
			csvErrors = append(csvErrors, CSVRowError{
				Line:    lineNum,
				Column:  "source_path",
				Message: "source_path is required",
			})
		}

		// Validate dest_path for operations that require it
		if (opType == "move" || opType == "flatten") && dest == "" {
			csvErrors = append(csvErrors, CSVRowError{
				Line:    lineNum,
				Column:  "dest_path",
				Message: fmt.Sprintf("dest_path is required for %s operation", opType),
			})
		}

		metadataStr := extractMetadataJSON(colIndex, record)
		rows = append(rows, rowData{opType: opType, source: source, dest: dest, metadata: metadataStr})
	}

	// If any errors, return without creating plan
	if len(csvErrors) > 0 {
		return &CSVImportResult{
			PlanID:     0,
			RowCount:   len(rows),
			ErrorCount: len(csvErrors),
			Errors:     csvErrors,
		}, nil
	}

	// Create plan
	desc := fmt.Sprintf("Imported from CSV (%d operations)", len(rows))
	planID, err := db.CreatePlan(database, planName, desc)
	if err != nil {
		return nil, fmt.Errorf("create plan from CSV: %w", err)
	}

	// Add operations
	for i, row := range rows {
		_, err := db.AddOperation(database, db.PlanOperation{
			PlanID:     planID,
			Seq:        i + 1,
			OpType:     row.opType,
			SourcePath: row.source,
			DestPath:   row.dest,
			Metadata:   row.metadata,
		})
		if err != nil {
			return nil, fmt.Errorf("add operation %d from CSV: %w", i+1, err)
		}
	}

	return &CSVImportResult{
		PlanID:     planID,
		RowCount:   len(rows),
		ErrorCount: 0,
		Errors:     nil,
	}, nil
}
