package annexvicheck

import (
	"strings"
	"testing"

	"github.com/cpouldev/go-clp/annexvi"
)

func TestReadCSVFindsHeaderAfterDisclaimer(t *testing.T) {
	fixture := `Unofficial convenience table

Index No,CAS No,Chemical Name,EC No,Hazard Class and Category Code(s),Classification Hazard Statement Code(s),M SCL ATE
001-001-00-9,1333-74-0,hydrogen,215-605-7,"Flam. Gas 1
Press. Gas",H220,
029-002-00-X,7440-50-8 [1],copper,231-159-6,"Aquatic Acute 1
Aquatic Chronic 1","H400
H410",M = 10
`

	records, err := ReadCSV(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("ReadCSV returned %d records, want 2", len(records))
	}
	if records[1].CASNumber != "7440-50-8" {
		t.Errorf("CASNumber = %q", records[1].CASNumber)
	}
}

func TestCompareSeparatesOrderFromSemanticDifferences(t *testing.T) {
	eurlex := []annexvi.Entry{
		{
			IndexNumber:          "001-001-00-9",
			ChemicalName:         "same",
			HazardClassCodes:     []string{"Skin Sens. 1", "Aquatic Chronic 1"},
			HazardStatementCodes: []string{"H317", "H410"},
		},
		{
			IndexNumber:          "002-002-00-8",
			ChemicalName:         "changed",
			HazardClassCodes:     []string{"Carc. 1B"},
			HazardStatementCodes: []string{"H350"},
		},
		{IndexNumber: "003-003-00-7", ChemicalName: "EUR-Lex only"},
	}
	echa := []Record{
		{
			IndexNumber:          "001-001-00-9",
			ChemicalName:         "same",
			HazardClassCodes:     []string{"Aquatic Chronic 1", "Skin Sens. 1"},
			HazardStatementCodes: []string{"H410", "H317"},
		},
		{
			IndexNumber:          "002-002-00-8",
			ChemicalName:         "changed",
			HazardClassCodes:     []string{"Carc. 2"},
			HazardStatementCodes: []string{"H351"},
		},
		{IndexNumber: "004-004-00-6", ChemicalName: "ECHA only"},
	}

	report := Compare(eurlex, echa)
	if report.Matched != 2 {
		t.Errorf("Matched = %d, want 2", report.Matched)
	}
	if len(report.OrderDifferences) != 1 || report.OrderDifferences[0].IndexNumber != "001-001-00-9" {
		t.Errorf("OrderDifferences = %+v", report.OrderDifferences)
	}
	if len(report.SemanticDifferences) != 1 || report.SemanticDifferences[0].IndexNumber != "002-002-00-8" {
		t.Errorf("SemanticDifferences = %+v", report.SemanticDifferences)
	}
	if strings.Join(report.OnlyEURLEX, ",") != "003-003-00-7" {
		t.Errorf("OnlyEURLEX = %v", report.OnlyEURLEX)
	}
	if strings.Join(report.OnlyECHA, ",") != "004-004-00-6" {
		t.Errorf("OnlyECHA = %v", report.OnlyECHA)
	}
}
