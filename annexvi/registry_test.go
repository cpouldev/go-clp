package annexvi

import (
	"testing"
	"time"
)

func TestEntriesAsOfApplicationDates(t *testing.T) {
	tests := []struct {
		name string
		date time.Time
		want int
	}{
		{"before supported snapshot", time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), 0},
		{"ATP22 snapshot", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 4419},
		{"before ATP23", time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC), 4419},
		{"ATP23 application in local zone", time.Date(2027, 2, 1, 0, 0, 0, 0, time.FixedZone("EET", 2*60*60)), 4441},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := EntriesAsOf(tt.date)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != tt.want {
				t.Fatalf("EntriesAsOf(%s) returned %d entries, want %d", tt.date.Format(time.DateOnly), len(entries), tt.want)
			}
		})
	}
}

func TestEmbeddedDatasetMetadataAndVersions(t *testing.T) {
	entries, err := loadEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4451 {
		t.Fatalf("embedded versioned entries = %d, want 4451", len(entries))
	}
	if DataVersion != "ATP23" || ConsolidationCELEX != "02008R1272-20260701" {
		t.Fatalf("metadata = %q/%q", DataVersion, ConsolidationCELEX)
	}
}

func TestATP23InsertAppearsOnlyFromApplicationDate(t *testing.T) {
	before, err := EntriesAsOf(time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	after, err := EntriesAsOf(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findIndex(before, "607-776-00-5"); ok {
		t.Fatal("ATP23 insertion is active before 2027-02-01")
	}
	entry, ok := findIndex(after, "607-776-00-5")
	if !ok {
		t.Fatal("ATP23 insertion missing on 2027-02-01")
	}
	if entry.Source != "32025R1222" {
		t.Fatalf("ATP23 source = %q", entry.Source)
	}
}

func TestEmbeddedEntryPreservesSourceFields(t *testing.T) {
	entries, err := EntriesAsOf(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	hydrogen, ok := findIndex(entries, "001-001-00-9")
	if !ok {
		t.Fatal("hydrogen entry not found")
	}
	if hydrogen.ChemicalName != "hydrogen" {
		t.Errorf("ChemicalName = %q", hydrogen.ChemicalName)
	}
	if len(hydrogen.CASNumbers) != 1 || hydrogen.CASNumbers[0] != "1333-74-0" {
		t.Errorf("CASNumbers = %#v", hydrogen.CASNumbers)
	}
	if len(hydrogen.Hazards) != 2 || hydrogen.Hazards[0].HCode != "H220" ||
		hydrogen.Hazards[1].Class != "Press. Gas" {
		t.Errorf("Hazards = %#v", hydrogen.Hazards)
	}
	if _, ok := findIndex(entries, "005-023-00-X"); !ok {
		t.Fatal("index number with X check character was not loaded")
	}
}

func TestAsOfBuildsEngineRegistryWithParsedSCLs(t *testing.T) {
	registry, err := AsOf(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	harmonised, ok := registry["7789-09-5"]
	if !ok {
		t.Fatal("CAS 7789-09-5 missing from engine registry")
	}
	limits := map[string]float64{}
	for _, limit := range harmonised.SCLs {
		limits[limit.HCode] = limit.Pct
	}
	if limits["H317"] != 0.2 || limits["H334"] != 0.2 || limits["H335"] != 5 {
		t.Fatalf("parsed SCLs = %#v", limits)
	}
	if harmonised.Source != "annex_vi/ATP23" {
		t.Fatalf("registry source = %q", harmonised.Source)
	}

	delete(registry, "7789-09-5")
	again, err := AsOf(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again["7789-09-5"]; !ok {
		t.Fatal("caller mutation leaked into the cached registry")
	}
}

func findIndex(entries []Entry, index string) (Entry, bool) {
	for _, entry := range entries {
		if entry.IndexNumber == index {
			return entry, true
		}
	}
	return Entry{}, false
}
