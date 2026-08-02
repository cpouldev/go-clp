package annexvisource

import (
	"strings"
	"testing"
	"time"
)

func TestParseXHTMLExtractsElevenColumnRowsAndStopsAtLegacyTable(t *testing.T) {
	fixture := `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
<p>Table 3</p><table><tbody>
<tr><td><p>Index No</p></td><td/><td/><td/><td/><td/><td/><td/><td/><td/><td/></tr>
<tr>
<td><p>▼M15</p><p>‘607-776-00-5</p></td>
<td><p>example substance</p></td><td><p>200-001-8</p></td>
<td><p>10043-35-3 [1]</p></td>
<td><p>Skin Sens. 1</p><p>Aquatic Chronic 1</p></td>
<td><p>H317</p><p>H410</p></td><td><p>GHS07</p></td>
<td><p>H317</p><p>H410</p></td><td><p>EUH071</p></td>
<td><p>H317: C ≥ 0,1 %</p><p>H410: M = 10</p></td><td><p>A</p></td>
</tr>
</tbody></table>
<p>Table 3.2</p><table><tbody><tr>
<td>999-999-99-9</td><td>legacy</td><td>-</td><td>-</td><td>x</td><td>x</td><td>x</td><td>x</td><td>x</td><td>x</td><td>x</td>
</tr></tbody></table></body></html>`

	records, err := ParseXHTML(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("ParseXHTML returned %d records, want 1", len(records))
	}
	record := records[0]
	if record.IndexNumber != "607-776-00-5" {
		t.Errorf("IndexNumber = %q", record.IndexNumber)
	}
	if record.CASNumber != "10043-35-3" {
		t.Errorf("CASNumber = %q", record.CASNumber)
	}
	if record.HazardClassCodes != "Skin Sens. 1\nAquatic Chronic 1" {
		t.Errorf("HazardClassCodes = %q", record.HazardClassCodes)
	}
	if record.SpecificLimits != "H317: C ≥ 0,1 %\nH410: M = 10" {
		t.Errorf("SpecificLimits = %q", record.SpecificLimits)
	}
}

func TestMergeAmendmentVersionsReplacesAndInserts(t *testing.T) {
	base := []VersionedRecord{
		{Record: record("001-001-00-9", "old A")},
		{Record: record("002-002-00-8", "old B")},
	}
	changes := []VersionedRecord{
		{Record: record("002-002-00-8", "new B")},
		{Record: record("003-003-00-7", "new C")},
	}
	baseDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	amendmentDate := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)

	got, err := MergeVersions(base, changes, baseDate, amendmentDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("MergeVersions returned %d rows, want 4", len(got))
	}

	oldB := findVersion(t, got, "002-002-00-8", "old B")
	if !oldB.AppliesFrom.Equal(baseDate) || oldB.AppliesUntil == nil || !oldB.AppliesUntil.Equal(amendmentDate) {
		t.Errorf("old replacement window = %v..%v", oldB.AppliesFrom, oldB.AppliesUntil)
	}
	newB := findVersion(t, got, "002-002-00-8", "new B")
	if !newB.AppliesFrom.Equal(amendmentDate) || newB.AppliesUntil != nil {
		t.Errorf("new replacement window = %v..%v", newB.AppliesFrom, newB.AppliesUntil)
	}
	newC := findVersion(t, got, "003-003-00-7", "new C")
	if !newC.AppliesFrom.Equal(amendmentDate) {
		t.Errorf("insert AppliesFrom = %v", newC.AppliesFrom)
	}
}

func TestMergeVersionsRejectsDuplicateAmendmentIndex(t *testing.T) {
	date := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	_, err := MergeVersions(nil, []VersionedRecord{
		{Record: record("001-001-00-9", "one")},
		{Record: record("001-001-00-9", "two")},
	}, date, date)
	if err == nil {
		t.Fatal("MergeVersions accepted duplicate amendment index")
	}
}

func record(index, name string) Record {
	return Record{IndexNumber: index, ChemicalName: name}
}

func findVersion(t *testing.T, records []VersionedRecord, index, name string) VersionedRecord {
	t.Helper()
	for _, record := range records {
		if record.IndexNumber == index && record.ChemicalName == name {
			return record
		}
	}
	t.Fatalf("version %s/%s not found", index, name)
	return VersionedRecord{}
}
