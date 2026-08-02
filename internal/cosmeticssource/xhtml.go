// Package cosmeticssource reads the pinned EUR-Lex XHTML used to generate the
// embedded fragrance-allergen registry.
package cosmeticssource

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const annexIIIID = "anx_III"

var (
	casPattern      = regexp.MustCompile(`\b[0-9]{2,7}-[0-9]{2}-[0-9]\b`)
	ecPattern       = regexp.MustCompile(`\b[0-9]{3}-[0-9]{3}-[0-9]\b`)
	modifierPattern = regexp.MustCompile(`[▼►]\s*(?:M[0-9]+|C[0-9]+|B)|◄`)
)

// Record is one Annex III entry subject to individual fragrance-allergen
// labelling. A reference may group several chemicals, INCI names, and CAS
// numbers.
type Record struct {
	ReferenceNumber string            `json:"referenceNumber"`
	ChemicalNames   []string          `json:"chemicalNames,omitempty"`
	INCINames       []string          `json:"inciNames,omitempty"`
	CASNumbers      []string          `json:"casNumbers,omitempty"`
	ECNumbers       []string          `json:"ecNumbers,omitempty"`
	INCIByCAS       map[string]string `json:"inciByCas,omitempty"`
}

type xmlRow struct {
	Cells []xmlCell `xml:"td"`
}

type xmlCell struct {
	Text    string
	Rowspan int
	Colspan int
}

func (cell *xmlCell) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	cell.Rowspan = intAttribute(start, "rowspan", 1)
	cell.Colspan = intAttribute(start, "colspan", 1)

	var builder strings.Builder
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
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
	cell.Text = cleanCell(builder.String())
	return nil
}

func intAttribute(start xml.StartElement, name string, fallback int) int {
	for _, attribute := range start.Attr {
		if attribute.Name.Local != name {
			continue
		}
		value, err := strconv.Atoi(attribute.Value)
		if err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

// ParseXHTML extracts the individually labelled fragrance allergens from Annex
// III of an EUR-Lex XHTML consolidation. It expands rowspans before grouping
// continuation rows, preserving every synonym and CAS number in grouped entries.
func ParseXHTML(reader io.Reader) ([]Record, error) {
	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("parse cosmetics XHTML: Annex III not found")
		}
		if err != nil {
			return nil, fmt.Errorf("parse cosmetics XHTML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "div" || attribute(start, "id") != annexIIIID {
			continue
		}
		rows, err := parseAnnex(decoder)
		if err != nil {
			return nil, err
		}
		return recordsFromRows(rows)
	}
}

func attribute(start xml.StartElement, name string) string {
	for _, attribute := range start.Attr {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func parseAnnex(decoder *xml.Decoder) ([]xmlRow, error) {
	depth := 1
	var rows []xmlRow
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("parse cosmetics Annex III: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == "tr" {
				var row xmlRow
				if err := decoder.DecodeElement(&row, &typed); err != nil {
					return nil, fmt.Errorf("decode cosmetics table row: %w", err)
				}
				rows = append(rows, row)
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return rows, nil
}

type span struct {
	text      string
	remaining int
}

func recordsFromRows(rows []xmlRow) ([]Record, error) {
	const columns = 9
	spans := make([]span, columns)
	byReference := make(map[string]*Record)
	var order []string

	for _, row := range rows {
		if len(row.Cells) == 1 && row.Cells[0].Colspan >= columns {
			for column := range spans {
				if spans[column].remaining > 0 {
					spans[column].remaining--
				}
			}
			continue
		}

		values := make([]string, columns)
		occupied := make([]bool, columns)
		for column := range spans {
			if spans[column].remaining == 0 {
				continue
			}
			values[column] = spans[column].text
			occupied[column] = true
			spans[column].remaining--
		}

		column := 0
		for _, cell := range row.Cells {
			for column < columns && occupied[column] {
				column++
			}
			for offset := 0; offset < cell.Colspan && column+offset < columns; offset++ {
				position := column + offset
				values[position] = cell.Text
				occupied[position] = true
				if cell.Rowspan > 1 {
					spans[position] = span{text: cell.Text, remaining: cell.Rowspan - 1}
				}
			}
			column += cell.Colspan
		}

		if !isIndividualLabellingCondition(values[7]) {
			continue
		}
		reference := strings.TrimSpace(values[0])
		if reference == "" || strings.EqualFold(reference, "Reference number") || reference == "a" {
			continue
		}
		record := byReference[reference]
		if record == nil {
			record = &Record{ReferenceNumber: reference}
			byReference[reference] = record
			order = append(order, reference)
		}
		record.ChemicalNames = appendUnique(record.ChemicalNames, cleanCell(values[1]))
		names := splitNames(values[2])
		for _, name := range names {
			record.INCINames = appendUnique(record.INCINames, name)
		}
		casNumbers := casPattern.FindAllString(values[3], -1)
		for _, cas := range casNumbers {
			record.CASNumbers = appendUnique(record.CASNumbers, cas)
		}
		pairINCINames(record, casNumbers, names)
		for _, ec := range ecPattern.FindAllString(values[4], -1) {
			record.ECNumbers = appendUnique(record.ECNumbers, ec)
		}
	}

	if len(order) == 0 {
		return nil, fmt.Errorf("parse cosmetics XHTML: no individually labelled Annex III entries found")
	}
	records := make([]Record, 0, len(order))
	for _, reference := range order {
		record := *byReference[reference]
		sort.Strings(record.CASNumbers)
		sort.Strings(record.ECNumbers)
		records = append(records, record)
	}
	return records, nil
}

func pairINCINames(record *Record, casNumbers, names []string) {
	if len(casNumbers) == 0 || len(names) == 0 {
		return
	}
	if record.INCIByCAS == nil {
		record.INCIByCAS = make(map[string]string)
	}
	for i, cas := range casNumbers {
		name := strings.TrimSpace(names[0])
		if len(names) == len(casNumbers) {
			name = strings.TrimSpace(names[i])
		}
		if _, exists := record.INCIByCAS[cas]; !exists {
			record.INCIByCAS[cas] = name
		}
	}
}

func isIndividualLabellingCondition(value string) bool {
	value = strings.ToLower(strings.ReplaceAll(value, "\u00a0", " "))
	return strings.Contains(value, "leave-on products") &&
		strings.Contains(value, "rinse-off products") &&
		(strings.Contains(value, "0,001") || strings.Contains(value, "0.001")) &&
		(strings.Contains(value, "0,01") || strings.Contains(value, "0.01"))
}

func splitNames(value string) []string {
	return strings.FieldsFunc(cleanCell(value), func(r rune) bool {
		return r == '\n' || r == ';'
	})
}

func appendUnique(values []string, value string) []string {
	value = strings.Trim(strings.TrimSpace(value), ";")
	if value == "" || value == "—" || value == "-" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
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
