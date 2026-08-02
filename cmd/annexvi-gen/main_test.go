package main

import (
	"testing"

	"github.com/cpouldev/go-clp/internal/annexvisource"
)

func TestBuildDatasetVersionsReplacementAndInsertion(t *testing.T) {
	base := []annexvisource.Record{
		{IndexNumber: "001-001-00-9", ChemicalName: "hydrogen", CASNumber: "1333-74-0"},
		{IndexNumber: "002-002-00-8", ChemicalName: "old", CASNumber: "100-00-1"},
	}
	changes := []annexvisource.Record{
		{IndexNumber: "002-002-00-8", ChemicalName: "new", CASNumber: "100-00-1"},
		{IndexNumber: "003-003-00-7", ChemicalName: "insert", CASNumber: "200-00-2"},
	}

	dataset, err := buildDataset(base, changes)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.Version != dataVersion || dataset.Consolidation != baseCELEX {
		t.Fatalf("metadata = %q/%q", dataset.Version, dataset.Consolidation)
	}
	if len(dataset.Rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(dataset.Rows))
	}

	assertRow := func(index, name, from, until, source string) {
		t.Helper()
		for _, row := range dataset.Rows {
			if row[0] == index && row[1] == name {
				if row[11] != from || row[12] != until || row[13] != source {
					t.Errorf("row %s/%s dates/source = %q/%q/%q", index, name, row[11], row[12], row[13])
				}
				return
			}
		}
		t.Errorf("row %s/%s not found", index, name)
	}
	assertRow("002-002-00-8", "old", "2026-05-01", "2027-02-01", baseCELEX)
	assertRow("002-002-00-8", "new", "2027-02-01", "", atp23CELEX)
	assertRow("003-003-00-7", "insert", "2027-02-01", "", atp23CELEX)
}

func TestValidateSourceCounts(t *testing.T) {
	if err := validateSourceCounts(make([]annexvisource.Record, expectedBaseRecords), make([]annexvisource.Record, expectedATP23Records)); err != nil {
		t.Fatalf("valid counts rejected: %v", err)
	}
	if err := validateSourceCounts(make([]annexvisource.Record, expectedBaseRecords-1), make([]annexvisource.Record, expectedATP23Records)); err == nil {
		t.Fatal("unexpected base count accepted")
	}
	if err := validateSourceCounts(make([]annexvisource.Record, expectedBaseRecords), make([]annexvisource.Record, expectedATP23Records-1)); err == nil {
		t.Fatal("unexpected ATP23 count accepted")
	}
}
