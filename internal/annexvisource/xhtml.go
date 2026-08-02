// Package annexvisource reads the pinned EUR-Lex XHTML used to generate the
// embedded Annex VI registry. It is internal so runtime users do not link the
// source reader.
package annexvisource

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cpouldev/go-clp/annexvi"
)

// Record is one Annex VI Table 3 source row.
type Record = annexvi.Record

// VersionedRecord is a source row with its half-open application interval.
type VersionedRecord struct {
	Record
	AppliesFrom  time.Time
	AppliesUntil *time.Time
}

var (
	indexNumberPattern = regexp.MustCompile(`[0-9]{3}-[0-9]{3}-[0-9]{2}-[0-9X]`)
	modifierPattern    = regexp.MustCompile(`[▼►]\s*(?:M[0-9]+|C[0-9]+|B)|◄`)
	casFootnotePattern = regexp.MustCompile(`\s*\[[^\]]*\]`)
)

type xmlRow struct {
	Cells []xmlCell `xml:"td"`
}

type xmlCell struct {
	text string
	rows []xmlRow
}

func (cell *xmlCell) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	var builder strings.Builder
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "tr" {
				var nested xmlRow
				if err := decoder.DecodeElement(&nested, &typed); err != nil {
					return err
				}
				cell.rows = append(cell.rows, nested)
				for _, nestedCell := range nested.Cells {
					if text := cleanCell(nestedCell.text); text != "" {
						builder.WriteString(text)
						builder.WriteByte('\n')
					}
				}
				continue
			}
			depth++
			if typed.Name.Local == "br" {
				builder.WriteByte('\n')
			}
		case xml.CharData:
			builder.Write(typed)
		case xml.EndElement:
			if typed.Name.Local == "p" || typed.Name.Local == "div" || typed.Name.Local == "li" {
				builder.WriteByte('\n')
			}
			depth--
		}
	}
	cell.text = builder.String()
	return nil
}

// ParseXHTML extracts uniform 11-column Annex VI records from an EUR-Lex XHTML
// document. Consolidation markers are removed and any legacy Table 3.2 rows are
// ignored.
func ParseXHTML(reader io.Reader) ([]Record, error) {
	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity

	var records []Record
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse EUR-Lex XHTML: %w", err)
		}
		switch typed := token.(type) {
		case xml.CharData:
			if len(records) > 0 && strings.Join(strings.Fields(string(typed)), " ") == "Table 3.2" {
				return records, nil
			}
		case xml.StartElement:
			if typed.Name.Local != "tr" {
				continue
			}
			var row xmlRow
			if err := decoder.DecodeElement(&row, &typed); err != nil {
				return nil, fmt.Errorf("decode EUR-Lex table row: %w", err)
			}
			for _, candidate := range flattenRows(row) {
				if record, ok := recordFromRow(candidate); ok {
					records = append(records, record)
				}
			}
		}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("parse EUR-Lex XHTML: no Annex VI Table 3 records found")
	}
	return records, nil
}

func flattenRows(row xmlRow) []xmlRow {
	rows := []xmlRow{row}
	for _, cell := range row.Cells {
		for _, nested := range cell.rows {
			rows = append(rows, flattenRows(nested)...)
		}
	}
	return rows
}

func recordFromRow(row xmlRow) (Record, bool) {
	if len(row.Cells) != 11 {
		return Record{}, false
	}
	values := make([]string, len(row.Cells))
	for i, cell := range row.Cells {
		values[i] = cleanCell(cell.text)
	}
	values[0] = indexNumberPattern.FindString(values[0])
	if values[0] == "" {
		return Record{}, false
	}
	values[3] = strings.TrimSpace(casFootnotePattern.ReplaceAllString(values[3], ""))
	return Record{
		IndexNumber:                      values[0],
		ChemicalName:                     values[1],
		ECNumber:                         values[2],
		CASNumber:                        values[3],
		HazardClassCodes:                 values[4],
		HazardStatementCodes:             values[5],
		PictogramSignalWordCodes:         values[6],
		LabelHazardStatementCodes:        values[7],
		SupplementalHazardStatementCodes: values[8],
		SpecificLimits:                   values[9],
		Notes:                            values[10],
	}, true
}

func cleanCell(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = modifierPattern.ReplaceAllString(value, "")
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Trim(strings.Join(lines, "\n"), "‘’")
}

// MergeVersions applies one future amendment snapshot to a consolidated base.
// Existing index numbers are retired at amendmentDate; new index numbers are
// inserted from that date.
func MergeVersions(base, changes []VersionedRecord, baseDate, amendmentDate time.Time) ([]VersionedRecord, error) {
	result := make([]VersionedRecord, len(base))
	baseIndex := make(map[string]int, len(base))
	for i, version := range base {
		if _, duplicate := baseIndex[version.IndexNumber]; duplicate {
			return nil, fmt.Errorf("duplicate base index number %s", version.IndexNumber)
		}
		version.AppliesFrom = baseDate
		version.AppliesUntil = nil
		result[i] = version
		baseIndex[version.IndexNumber] = i
	}

	seenChanges := make(map[string]bool, len(changes))
	for _, version := range changes {
		if seenChanges[version.IndexNumber] {
			return nil, fmt.Errorf("duplicate amendment index number %s", version.IndexNumber)
		}
		seenChanges[version.IndexNumber] = true
		if basePosition, replacing := baseIndex[version.IndexNumber]; replacing {
			until := amendmentDate
			result[basePosition].AppliesUntil = &until
		}
		version.AppliesFrom = amendmentDate
		version.AppliesUntil = nil
		result = append(result, version)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IndexNumber != result[j].IndexNumber {
			return result[i].IndexNumber < result[j].IndexNumber
		}
		return result[i].AppliesFrom.Before(result[j].AppliesFrom)
	})
	return result, nil
}
