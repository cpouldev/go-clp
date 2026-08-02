package annexvi

import (
	"regexp"
	"strconv"
	"strings"
)

// SCL is a substance-specific concentration limit for one hazard statement.
type SCL struct {
	HCode string  `json:"hCode"`
	Pct   float64 `json:"pct"`
}

// HarmonisedHazard is one hazard class, category, and statement tuple from
// Annex VI Table 3.
type HarmonisedHazard struct {
	HCode    string `json:"hCode"`
	Class    string `json:"class"`
	Category string `json:"category"`
}

// Record is one source row from Annex VI Table 3.
type Record struct {
	IndexNumber                      string
	ChemicalName                     string
	ECNumber                         string
	CASNumber                        string
	HazardClassCodes                 string
	HazardStatementCodes             string
	PictogramSignalWordCodes         string
	LabelHazardStatementCodes        string
	SupplementalHazardStatementCodes string
	SpecificLimits                   string
	Notes                            string
}

// ParsedRecord contains the structured limits and hazards parsed from a Record.
type ParsedRecord struct {
	IndexNumber    string
	SCLs           []SCL
	MFactorAcute   *int
	MFactorChronic *int
	Hazards        []HarmonisedHazard
}

var (
	hazardConditionPattern = regexp.MustCompile(`(?i)\b(H\d{3}[A-Za-z]*)\b\s*:\s*([^;]+)`)
	lowerBoundPattern      = regexp.MustCompile(`C\s*>=\s*([0-9]+(?:\.[0-9]+)?)\s*%`)
	rangeLowerPattern      = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*%\s*<=\s*C`)
	percentagePattern      = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*%`)
	acuteMFactorPattern    = regexp.MustCompile(`(?i)M\s*\(\s*acute\s*\)\s*=\s*([0-9]+)`)
	chronicMFactorPattern  = regexp.MustCompile(`(?i)M\s*\(\s*chronic\s*\)\s*=\s*([0-9]+)`)
	bareMFactorPattern     = regexp.MustCompile(`(?i)(?:^|[^a-z(])M\s*=\s*([0-9]+)`)
	acuteMarkerPattern     = regexp.MustCompile(`(?i)\bH400\b|aquatic\s+acute`)
	chronicMarkerPattern   = regexp.MustCompile(`(?i)\bH410\b|aquatic\s+chronic`)
	categoryPattern        = regexp.MustCompile(`^([0-9][A-Z]?)$`)
	decimalCommaPattern    = regexp.MustCompile(`([0-9]),([0-9])`)
	casFootnotePattern     = regexp.MustCompile(`\s*\[[^\]]*\]`)
)

func normalizeLimits(value string) string {
	value = strings.ReplaceAll(value, "≥", ">=")
	value = strings.ReplaceAll(value, "≤", "<=")
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = decimalCommaPattern.ReplaceAllString(value, "$1.$2")
	return strings.Join(strings.Fields(value), " ")
}

// ParseSpecificLimits parses the free-text SCL/M-factor/ATE column. Unknown
// segments and ATE values are left untouched because they do not override the
// engine's concentration limits or aquatic M-factors.
func ParseSpecificLimits(raw string) (scls []SCL, acute, chronic *int) {
	text := normalizeLimits(raw)
	if text == "" {
		return nil, nil, nil
	}
	setAcute := func(value int) {
		if acute == nil {
			acute = &value
		}
	}
	setChronic := func(value int) {
		if chronic == nil {
			chronic = &value
		}
	}

	if value, ok := firstInt(acuteMFactorPattern, text); ok {
		setAcute(value)
	}
	if value, ok := firstInt(chronicMFactorPattern, text); ok {
		setChronic(value)
	}

	for _, match := range hazardConditionPattern.FindAllStringSubmatch(text, -1) {
		if percentage, ok := percentageFromCondition(match[2]); ok {
			scls = append(scls, SCL{HCode: strings.ToUpper(match[1]), Pct: percentage})
		}
	}

	for _, segment := range strings.Split(text, ";") {
		value, ok := firstInt(bareMFactorPattern, segment)
		if !ok {
			continue
		}
		switch {
		case acuteMarkerPattern.MatchString(segment):
			setAcute(value)
		case chronicMarkerPattern.MatchString(segment):
			setChronic(value)
		}
	}

	if acute == nil && chronic == nil {
		if value, ok := firstInt(bareMFactorPattern, text); ok {
			hasAcute := acuteMarkerPattern.MatchString(text)
			hasChronic := chronicMarkerPattern.MatchString(text)
			switch {
			case hasChronic && !hasAcute:
				setChronic(value)
			case hasAcute && !hasChronic:
				setAcute(value)
			default:
				setAcute(value)
				setChronic(value)
			}
		}
	}
	return scls, acute, chronic
}

func percentageFromCondition(condition string) (float64, bool) {
	if match := lowerBoundPattern.FindStringSubmatch(condition); match != nil {
		return parseFloat(match[1])
	}
	if match := rangeLowerPattern.FindStringSubmatch(condition); match != nil {
		return parseFloat(match[1])
	}
	if match := percentagePattern.FindStringSubmatch(condition); match != nil {
		return parseFloat(match[1])
	}
	return 0, false
}

// ParseHazards pairs the classification and hazard-statement columns in source
// order. If a row is ragged, only pairs present in both columns are returned.
func ParseHazards(classCodes, statementCodes string) []HarmonisedHazard {
	classes := splitList(classCodes)
	statements := splitList(statementCodes)

	// Table 3 lists the class and statement columns in corresponding order, but
	// only while every class carries a statement. Where the counts differ — e.g.
	// index 005-001-00-X is [Press. Gas, Acute Tox. 2 *, Skin Corr. 1A] against
	// [H330, H314], because Press. Gas has no hazard statement of its own —
	// position no longer identifies the pair, and zipping the two columns files
	// every statement under the wrong class.
	//
	// The classification (class + category) is the legally decisive part, so it
	// is always recorded in full. The statement is then walked alongside it and
	// attached only where the two agree on the CLP hazard group, which realigns
	// the columns across a class that has no statement.
	aligned := len(classes) == len(statements)

	hazards := make([]HarmonisedHazard, 0, len(classes))
	next := 0
	for index, code := range classes {
		class, category := splitClassCategory(code)
		if class == "" && category == "" {
			continue
		}
		hazard := HarmonisedHazard{Class: class, Category: category}
		switch {
		case aligned:
			hazard.HCode = strings.TrimSpace(statements[index])
		case next < len(statements) && hazardGroup(class) == statementGroup(statements[next]):
			hazard.HCode = strings.TrimSpace(statements[next])
			next++
		}
		hazards = append(hazards, hazard)
	}
	return hazards
}

// hazardGroup buckets a Table 3 hazard class abbreviation into its CLP group.
// Together with statementGroup it is enough to realign the class and statement
// columns of an uneven row without a full class-to-statement table.
func hazardGroup(class string) string {
	key := strings.ToLower(strings.TrimSpace(class))
	if physicalHazardClasses[key] {
		return "physical"
	}
	if environmentalHazardClasses[key] {
		return "environmental"
	}
	return "health"
}

// statementGroup buckets a hazard statement by its series: H2xx are physical
// hazards, H3xx health hazards, and H4xx environmental hazards.
func statementGroup(statement string) string {
	code := strings.ToUpper(strings.TrimSpace(statement))
	if len(code) < 2 || code[0] != 'H' {
		return ""
	}
	switch code[1] {
	case '2':
		return "physical"
	case '3':
		return "health"
	case '4':
		return "environmental"
	}
	return ""
}

var physicalHazardClasses = map[string]bool{
	"unst. expl.": true, "expl.": true, "flam. gas": true, "chem. unst. gas": true,
	"aerosol": true, "flam. aerosol": true, "ox. gas": true, "press. gas": true,
	"flam. liq.": true, "flam. sol.": true, "self-react.": true, "pyr. liq.": true,
	"pyr. sol.": true, "self-heat.": true, "water-react.": true, "ox. liq.": true,
	"ox. sol.": true, "org. perox.": true, "met. corr.": true, "des. expl.": true,
}

var environmentalHazardClasses = map[string]bool{
	"aquatic acute": true, "aquatic chronic": true, "ozone": true,
}

// ParseRecord parses all structured CLP fields carried by a source row.
func ParseRecord(record Record) ParsedRecord {
	scls, acute, chronic := ParseSpecificLimits(record.SpecificLimits)
	return ParsedRecord{
		IndexNumber:    strings.TrimSpace(record.IndexNumber),
		SCLs:           scls,
		MFactorAcute:   acute,
		MFactorChronic: chronic,
		Hazards:        ParseHazards(record.HazardClassCodes, record.HazardStatementCodes),
	}
}

func splitClassCategory(code string) (class, category string) {
	fields := strings.Fields(strings.TrimSpace(code))

	// Table 3 appends the Note reference to the class code ("Acute Tox. 4 *").
	// The asterisk is a footnote marker, not part of the classification, and
	// leaving it attached hides the category from the parser — which would carry
	// ~2,350 harmonised entries with no category at all.
	cleaned := fields[:0]
	for _, field := range fields {
		if field = strings.TrimRight(field, "*"); field != "" {
			cleaned = append(cleaned, field)
		}
	}
	if fields = cleaned; len(fields) == 0 {
		return "", ""
	}

	last := fields[len(fields)-1]
	if categoryPattern.MatchString(last) {
		return strings.Join(fields[:len(fields)-1], " "), last
	}
	return strings.Join(fields, " "), ""
}

func splitList(value string) []string {
	var result []string
	for _, part := range strings.FieldsFunc(value, func(char rune) bool {
		return char == ';' || char == '\n' || char == '\r'
	}) {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func cleanCAS(value string) string {
	return strings.TrimSpace(casFootnotePattern.ReplaceAllString(value, ""))
}

func firstInt(pattern *regexp.Regexp, value string) (int, bool) {
	if match := pattern.FindStringSubmatch(value); match != nil {
		parsed, err := strconv.Atoi(match[1])
		return parsed, err == nil
	}
	return 0, false
}

func parseFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil
}
