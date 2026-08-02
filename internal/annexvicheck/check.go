// Package annexvicheck compares a caller-provided ECHA convenience export with
// the authoritative EUR-Lex data embedded by annexvi. It contains no ECHA data
// and performs no network access.
package annexvicheck

import (
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/cpouldev/go-clp/annexvi"
)

var casPattern = regexp.MustCompile(`\b[0-9]{2,7}-[0-9]{2}-[0-9]\b`)

// Record is the subset of one ECHA convenience-export row used by the private
// comparison.
type Record struct {
	IndexNumber          string
	CASNumber            string
	ChemicalName         string
	ECNumber             string
	HazardClassCodes     []string
	HazardStatementCodes []string
	SpecificLimits       string
}

// Difference describes differing hazard class or statement sequences for one
// shared Annex VI index number.
type Difference struct {
	IndexNumber      string
	EURLEXClasses    []string
	ECHAClasses      []string
	EURLEXStatements []string
	ECHAStatements   []string
}

// Report separates harmless ordering differences from substantive differences
// and lists entries present in only one source.
type Report struct {
	EURLEXCount         int
	ECHACount           int
	Matched             int
	OrderDifferences    []Difference
	SemanticDifferences []Difference
	OnlyEURLEX          []string
	OnlyECHA            []string
}

// ReadCSV reads a CSV exported manually from ECHA's Annex VI convenience
// workbook. It locates the header after any disclaimer preamble and ignores rows
// without an Annex VI index number.
func ReadCSV(reader io.Reader) ([]Record, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("annexvicheck: read CSV: %w", err)
	}

	headerRow := -1
	columns := map[string]int{}
	for i, row := range rows {
		if i >= 15 {
			break
		}
		candidate := mapColumns(row)
		if _, hasIndex := candidate["index"]; hasIndex {
			if _, hasClass := candidate["class"]; hasClass {
				headerRow = i
				columns = candidate
				break
			}
		}
	}
	if headerRow < 0 {
		return nil, fmt.Errorf("annexvicheck: no recognised header in the first 15 CSV rows")
	}

	value := func(row []string, key string) string {
		column, ok := columns[key]
		if !ok || column >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[column])
	}
	seen := make(map[string]bool)
	var records []Record
	for _, row := range rows[headerRow+1:] {
		index := value(row, "index")
		if index == "" {
			continue
		}
		if seen[index] {
			return nil, fmt.Errorf("annexvicheck: duplicate index number %s", index)
		}
		seen[index] = true
		cas := casPattern.FindString(value(row, "cas"))
		records = append(records, Record{
			IndexNumber:          index,
			CASNumber:            cas,
			ChemicalName:         value(row, "name"),
			ECNumber:             value(row, "ec"),
			HazardClassCodes:     splitCodes(value(row, "class")),
			HazardStatementCodes: splitCodes(value(row, "statement")),
			SpecificLimits:       value(row, "limits"),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].IndexNumber < records[j].IndexNumber })
	return records, nil
}

func mapColumns(header []string) map[string]int {
	columns := make(map[string]int)
	for i, raw := range header {
		normalized := normalizeHeader(raw)
		switch {
		case normalized == "indexno" || normalized == "indexnumber":
			columns["index"] = i
		case normalized == "casno" || normalized == "casnumber":
			columns["cas"] = i
		case normalized == "chemicalname" || normalized == "name":
			columns["name"] = i
		case normalized == "ecno" || normalized == "ecnumber":
			columns["ec"] = i
		case strings.Contains(normalized, "hazardclassandcategorycode"):
			columns["class"] = i
		case strings.Contains(normalized, "classificationhazardstatementcode"):
			columns["statement"] = i
		case normalized == "msclate" || strings.Contains(normalized, "specificconcentrationlimit"):
			columns["limits"] = i
		}
	}
	return columns
}

func normalizeHeader(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func splitCodes(value string) []string {
	var result []string
	for _, line := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' || r == ';' }) {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// Compare compares ECHA records with an EUR-Lex snapshot by Annex VI index
// number. Hazard order is reported separately because ECHA commonly reorders
// legally equivalent class and statement lists.
func Compare(eurlex []annexvi.Entry, echa []Record) Report {
	report := Report{EURLEXCount: len(eurlex), ECHACount: len(echa)}
	eurlexByIndex := make(map[string]annexvi.Entry, len(eurlex))
	for _, entry := range eurlex {
		eurlexByIndex[entry.IndexNumber] = entry
	}
	echaByIndex := make(map[string]Record, len(echa))
	for _, record := range echa {
		echaByIndex[record.IndexNumber] = record
	}

	for index, entry := range eurlexByIndex {
		record, exists := echaByIndex[index]
		if !exists {
			report.OnlyEURLEX = append(report.OnlyEURLEX, index)
			continue
		}
		report.Matched++
		if equalStrings(entry.HazardClassCodes, record.HazardClassCodes) &&
			equalStrings(entry.HazardStatementCodes, record.HazardStatementCodes) {
			continue
		}
		difference := Difference{
			IndexNumber:      index,
			EURLEXClasses:    append([]string(nil), entry.HazardClassCodes...),
			ECHAClasses:      append([]string(nil), record.HazardClassCodes...),
			EURLEXStatements: append([]string(nil), entry.HazardStatementCodes...),
			ECHAStatements:   append([]string(nil), record.HazardStatementCodes...),
		}
		if equalSets(entry.HazardClassCodes, record.HazardClassCodes) &&
			equalSets(entry.HazardStatementCodes, record.HazardStatementCodes) {
			report.OrderDifferences = append(report.OrderDifferences, difference)
		} else {
			report.SemanticDifferences = append(report.SemanticDifferences, difference)
		}
	}
	for index := range echaByIndex {
		if _, exists := eurlexByIndex[index]; !exists {
			report.OnlyECHA = append(report.OnlyECHA, index)
		}
	}
	sort.Strings(report.OnlyEURLEX)
	sort.Strings(report.OnlyECHA)
	sort.Slice(report.OrderDifferences, func(i, j int) bool {
		return report.OrderDifferences[i].IndexNumber < report.OrderDifferences[j].IndexNumber
	})
	sort.Slice(report.SemanticDifferences, func(i, j int) bool {
		return report.SemanticDifferences[i].IndexNumber < report.SemanticDifferences[j].IndexNumber
	})
	return report
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i]) != strings.TrimSpace(right[i]) {
			return false
		}
	}
	return true
}

func equalSets(left, right []string) bool {
	left = normalizedSet(left)
	right = normalizedSet(right)
	return equalStrings(left, right)
}

func normalizedSet(values []string) []string {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
