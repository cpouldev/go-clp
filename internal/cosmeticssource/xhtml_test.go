package cosmeticssource

import (
	"os"
	"strings"
	"testing"
)

func TestParseXHTMLCarriesRowspansAcrossAllergenContinuationRows(t *testing.T) {
	fixture := `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
<div id="anx_III"><table><tbody>
<tr>
  <td rowspan="2">364</td><td>chemical one</td><td>Alpha One</td><td>111-11-1</td><td>200-001-1</td>
  <td rowspan="2"></td><td rowspan="2"></td>
  <td rowspan="2">The presence of the substance shall be indicated when its concentration exceeds:
    <br/>0,001 % in leave-on products<br/>0,01 % in rinse-off products.</td>
  <td rowspan="2"></td>
</tr>
<tr><td>chemical two</td><td>Alpha Two</td><td>222-22-2</td><td>200-002-2</td></tr>
<tr><td>365</td><td>not an allergen</td><td>Other</td><td>333-33-3</td><td></td><td></td><td></td><td>unrelated restriction</td><td></td></tr>
</tbody></table></div>
<div id="anx_IV"><table><tbody><tr><td>999</td></tr></tbody></table></div>
</body></html>`

	records, err := ParseXHTML(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("ParseXHTML returned %d records, want 1", len(records))
	}
	record := records[0]
	if record.ReferenceNumber != "364" {
		t.Errorf("ReferenceNumber = %q", record.ReferenceNumber)
	}
	if got := strings.Join(record.INCINames, ","); got != "Alpha One,Alpha Two" {
		t.Errorf("INCINames = %q", got)
	}
	if got := strings.Join(record.CASNumbers, ","); got != "111-11-1,222-22-2" {
		t.Errorf("CASNumbers = %q", got)
	}
	if got := record.INCIByCAS["111-11-1"]; got != "Alpha One" {
		t.Errorf("INCIByCAS[111-11-1] = %q", got)
	}
	if got := record.INCIByCAS["222-22-2"]; got != "Alpha Two" {
		t.Errorf("INCIByCAS[222-22-2] = %q", got)
	}
}

func TestParseOfficialXHTML(t *testing.T) {
	path := os.Getenv("COSMETICS_XHTML")
	if path == "" {
		t.Skip("set COSMETICS_XHTML to run the pinned-source check")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	records, err := ParseXHTML(file)
	if err != nil {
		t.Fatal(err)
	}
	var references []string
	cas := make(map[string]bool)
	inciCount := 0
	for _, record := range records {
		references = append(references, record.ReferenceNumber)
		inciCount += len(record.INCINames)
		for _, number := range record.CASNumbers {
			cas[number] = true
		}
	}
	t.Logf("references: %s", strings.Join(references, ", "))
	if len(records) != 81 {
		t.Errorf("entries = %d, want 81", len(records))
	}
	if len(cas) != 167 {
		t.Errorf("CAS numbers = %d, want 167", len(cas))
	}
	// The Formex and XHTML representations contain the same 140 distinct
	// Common Ingredients Glossary names. Counting expanded rowspans instead
	// incorrectly counts product-restriction rows as additional names.
	if inciCount != 140 {
		t.Errorf("distinct INCI names = %d, want 140", inciCount)
	}
}
