// Package annexvidata defines the compact, versioned wire format shared by the
// Annex VI generator and runtime loader.
package annexvidata

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Columns is the fixed column order in Dataset.Rows.
var Columns = []string{
	"index_no",
	"chemical_name",
	"ec_no",
	"cas_no",
	"hazard_class_codes",
	"hazard_statement_codes",
	"pictogram_signal_word_codes",
	"label_hazard_statement_codes",
	"supplemental_hazard_statement_codes",
	"specific_limits",
	"notes",
	"applies_from",
	"applies_until",
	"source",
}

// Dataset is the columnar JSON document stored inside the embedded gzip file.
type Dataset struct {
	Version       string     `json:"version"`
	Consolidation string     `json:"consolidation"`
	Sources       []string   `json:"sources"`
	Columns       []string   `json:"columns"`
	Rows          [][]string `json:"rows"`
}

// EncodeGZIP writes a reproducible best-compression gzip stream.
func EncodeGZIP(writer io.Writer, dataset Dataset) error {
	gzipWriter, err := gzip.NewWriterLevel(writer, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	encoder := json.NewEncoder(gzipWriter)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(dataset); err != nil {
		_ = gzipWriter.Close()
		return fmt.Errorf("encode Annex VI dataset: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("close Annex VI gzip stream: %w", err)
	}
	return nil
}

// DecodeGZIP reads and validates the embedded dataset schema.
func DecodeGZIP(reader io.Reader) (Dataset, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return Dataset{}, fmt.Errorf("open Annex VI gzip stream: %w", err)
	}
	defer gzipReader.Close()

	var dataset Dataset
	if err := json.NewDecoder(gzipReader).Decode(&dataset); err != nil {
		return Dataset{}, fmt.Errorf("decode Annex VI dataset: %w", err)
	}
	if !reflect.DeepEqual(dataset.Columns, Columns) {
		return Dataset{}, fmt.Errorf("unsupported Annex VI column schema %q", dataset.Columns)
	}
	for index, row := range dataset.Rows {
		if len(row) != len(Columns) {
			return Dataset{}, fmt.Errorf("Annex VI row %d has %d columns, want %d", index, len(row), len(Columns))
		}
	}
	return dataset, nil
}
