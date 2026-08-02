package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDatasetCreatesReadableArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "allergens.json.gz")
	if err := writeDataset(path, dataset{CELEX: defaultCELEX}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("generated mode = %o, want 644", got)
	}
}
