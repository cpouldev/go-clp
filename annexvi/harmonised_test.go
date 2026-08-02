package annexvi

import (
	"testing"
	"time"
)

// harmonised_test.go covers the two columns of Table 3 that carry the
// classification itself: how a hazard class is paired with its hazard statement,
// and how the parsed result reaches the engine registry.

// TestParseHazards_UnevenColumnsAlignByGroup covers a row whose class and
// statement columns have different lengths. Index 005-001-00-X (boron
// trifluoride) is [Press. Gas, Acute Tox. 2 *, Skin Corr. 1A] against
// [H330, H314], because Press. Gas carries no hazard statement. Zipping the
// columns by position files H330 ("Fatal if inhaled") under Press. Gas and H314
// ("severe skin burns") under Acute Tox., and drops Skin Corr. 1A entirely.
func TestParseHazards_UnevenColumnsAlignByGroup(t *testing.T) {
	got := ParseHazards("Press. Gas; Acute Tox. 2 *; Skin Corr. 1A", "H330; H314")

	want := []HarmonisedHazard{
		{Class: "Press. Gas"},
		{HCode: "H330", Class: "Acute Tox.", Category: "2"},
		{HCode: "H314", Class: "Skin Corr.", Category: "1A"},
	}
	if len(got) != len(want) {
		t.Fatalf("ParseHazards = %#v, want %d hazards", got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hazard %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// TestParseHazards_StripsNoteMarkerFromCategory covers the Note reference Table 3
// appends to a class code. The asterisk is a footnote marker, not part of the
// classification, and leaving it attached hides the category.
func TestParseHazards_StripsNoteMarkerFromCategory(t *testing.T) {
	for _, code := range []string{"Acute Tox. 4 *", "Acute Tox. 4*", "Acute Tox. 4"} {
		got := ParseHazards(code, "H302")
		if len(got) != 1 {
			t.Fatalf("ParseHazards(%q) = %#v, want 1 hazard", code, got)
		}
		if got[0].Class != "Acute Tox." || got[0].Category != "4" {
			t.Errorf("ParseHazards(%q) = %#v, want class %q category %q", code, got[0], "Acute Tox.", "4")
		}
	}
}

// TestParseHazards_PreservesStatementSuffixCase covers the one place where a
// hazard statement's final-letter CASE changes its meaning: H360FD is Repr. 1 for
// both endpoints, H360Fd is Repr. 1 + Repr. 2. Upper-casing the source destroys
// the distinction before the phrase library ever sees it.
func TestParseHazards_PreservesStatementSuffixCase(t *testing.T) {
	for _, code := range []string{"H360FD", "H360Fd", "H360Df", "H350i"} {
		got := ParseHazards("Repr. 1B", code)
		if len(got) != 1 {
			t.Fatalf("ParseHazards(_, %q) = %#v, want 1 hazard", code, got)
		}
		if got[0].HCode != code {
			t.Errorf("HCode = %q, want %q verbatim", got[0].HCode, code)
		}
	}
}

// TestAsOf_CarriesClassificationAndECKeys covers what the engine registry
// receives: the harmonised classification itself (CLP Art. 4(3) makes it
// mandatory, so discarding it makes an Annex VI match contribute nothing), and
// an index entry for every identifier the row carries.
func TestAsOf_CarriesClassificationAndECKeys(t *testing.T) {
	registry, err := AsOf(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AsOf: %v", err)
	}

	// Trimethyl borate is harmonised Repr. 1B / H360FD.
	entry, ok := registry["121-43-7"]
	if !ok {
		t.Fatal("CAS 121-43-7 missing from the registry")
	}
	var repr bool
	for _, hazard := range entry.Hazards {
		if hazard.Class == "Repr." && hazard.Category == "1B" {
			repr = true
		}
	}
	if !repr {
		t.Errorf("harmonised classification not carried; Hazards = %#v", entry.Hazards)
	}

	// Every applicable entry must be reachable by each identifier it lists —
	// several hundred carry an EC number and no CAS.
	entries, err := EntriesAsOf(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EntriesAsOf: %v", err)
	}
	var ecOnly, reachable int
	for _, e := range entries {
		if len(e.CASNumbers) != 0 || len(e.ECNumbers) == 0 {
			continue
		}
		ecOnly++
		if _, ok := registry[e.ECNumbers[0]]; ok {
			reachable++
		}
	}
	if ecOnly == 0 {
		t.Fatal("no EC-only entries in the snapshot; the guard would be vacuous")
	}
	if reachable != ecOnly {
		t.Errorf("%d of %d EC-only entries are unreachable in the registry", ecOnly-reachable, ecOnly)
	}
}
