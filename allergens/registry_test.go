package allergens

import (
	"slices"
	"testing"
)

func TestLoadProvidesAnnexIIIRegistry(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(registry.Entries); got != 81 {
		t.Fatalf("Load returned %d entries, want 81", got)
	}
	if got := len(registry.CAS); got != 167 {
		t.Fatalf("Load returned %d CAS numbers, want 167", got)
	}
	inciCount := 0
	inciNames := make(map[string]bool)
	for _, entry := range registry.Entries {
		inciCount += len(entry.INCINames)
		for _, name := range entry.INCINames {
			inciNames[name] = true
		}
	}
	if inciCount != 140 {
		t.Fatalf("Load returned %d distinct INCI names, want 140", inciCount)
	}
	if len(inciNames) != 140 {
		t.Fatalf("Load returned %d unique INCI names, want 140", len(inciNames))
	}
	if !registry.CAS["100-51-6"] {
		t.Error("benzyl alcohol CAS is missing")
	}
	if got := registry.INCIByCAS["100-51-6"]; got != "Benzyl Alcohol" {
		t.Errorf("benzyl alcohol INCI = %q", got)
	}
	if got := registry.INCIByCAS["5989-27-5"]; got != "Limonene" {
		t.Errorf("d-limonene INCI = %q", got)
	}
}

func TestLoadIncludes2025CorrigendumNames(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]Entry, len(registry.Entries))
	for _, entry := range registry.Entries {
		entries[entry.ReferenceNumber] = entry
	}

	checks := []struct {
		reference string
		name      string
	}{
		{reference: "157", name: "Rose ketone 4 (Damascenone)"},
		{reference: "364", name: "Pelargonium Graveolens Leaf Oil"},
		{reference: "365", name: "Pogostemon Cablin Leaf Oil"},
	}
	for _, check := range checks {
		if !slices.Contains(entries[check.reference].INCINames, check.name) {
			t.Errorf("entry %s INCI names %q do not contain %q", check.reference, entries[check.reference].INCINames, check.name)
		}
	}
}

func TestLoadReturnsIndependentRegistry(t *testing.T) {
	first, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	first.CAS["100-51-6"] = false
	first.Entries[0].CASNumbers[0] = "changed"

	second, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !second.CAS["100-51-6"] {
		t.Error("mutating a loaded CAS map changed the embedded registry")
	}
	if second.Entries[0].CASNumbers[0] == "changed" {
		t.Error("mutating a loaded entry changed the embedded registry")
	}
}

func TestRequiresDisclosureUsesStrictCosmeticsThresholds(t *testing.T) {
	tests := []struct {
		name          string
		kind          ProductKind
		concentration float64
		want          bool
	}{
		{name: "leave-on at threshold", kind: LeaveOn, concentration: 0.001, want: false},
		{name: "leave-on over threshold", kind: LeaveOn, concentration: 0.0011, want: true},
		{name: "rinse-off at threshold", kind: RinseOff, concentration: 0.01, want: false},
		{name: "rinse-off over threshold", kind: RinseOff, concentration: 0.0101, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequiresDisclosure(tt.kind, tt.concentration)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("RequiresDisclosure(%q, %g) = %v, want %v", tt.kind, tt.concentration, got, tt.want)
			}
		})
	}
}

func TestRequiresDisclosureRejectsInvalidInput(t *testing.T) {
	if _, err := RequiresDisclosure(ProductKind("aerosol"), 0.1); err == nil {
		t.Error("RequiresDisclosure accepted an unknown product kind")
	}
	if _, err := RequiresDisclosure(LeaveOn, -0.1); err == nil {
		t.Error("RequiresDisclosure accepted a negative concentration")
	}
}
