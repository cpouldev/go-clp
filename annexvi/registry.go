package annexvi

import (
	"bytes"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	clp "github.com/cpouldev/go-clp"
	"github.com/cpouldev/go-clp/internal/annexvidata"
)

const (
	// DataVersion identifies the latest adaptation to technical progress carried
	// by the embedded registry.
	DataVersion = "ATP23"

	// ConsolidationCELEX is the immutable consolidated CLP source used as the
	// ATP22 baseline.
	ConsolidationCELEX = "02008R1272-20260701"

	// MinimumSupportedDate is the first date represented by the embedded
	// snapshot. Earlier dates return an empty result.
	MinimumSupportedDate = "2026-05-01"

	// ATP23ApplicationDate is the mandatory application date for Regulation
	// (EU) 2025/1222. The voluntary early-application option is not inferred.
	ATP23ApplicationDate = "2027-02-01"
)

// Entry is one versioned Annex VI Table 3 entry.
type Entry struct {
	IndexNumber                      string
	ChemicalName                     string
	ECNumbers                        []string
	CASNumbers                       []string
	HazardClassCodes                 []string
	HazardStatementCodes             []string
	PictogramSignalWordCodes         []string
	LabelHazardStatementCodes        []string
	SupplementalHazardStatementCodes []string
	SpecificLimits                   string
	Notes                            []string
	SCLs                             []SCL
	MFactorAcute                     int
	MFactorChronic                   int
	Hazards                          []HarmonisedHazard
	AppliesFrom                      time.Time
	AppliesUntil                     *time.Time
	Source                           string
}

//go:generate go run ../cmd/annexvi-gen -out data/annexvi.json.gz

//go:embed data/annexvi.json.gz
var embeddedDataset []byte

var (
	entryOnce  sync.Once
	entries    []Entry
	entriesErr error
	casPattern = regexp.MustCompile(`^[0-9]{2,7}-[0-9]{2}-[0-9]$`)
)

// EntriesAsOf returns independent copies of all Table 3 entries applicable on
// the supplied civil date. The supported snapshot begins on 2026-05-01; ATP23
// insertions and replacements become active on 2027-02-01.
func EntriesAsOf(date time.Time) ([]Entry, error) {
	all, err := loadEntries()
	if err != nil {
		return nil, err
	}
	day := civilDate(date)
	result := make([]Entry, 0, len(all))
	for _, entry := range all {
		if !entryAppliesOn(entry, day) {
			continue
		}
		result = append(result, cloneEntry(entry))
	}
	return result, nil
}

// AsOf returns a CAS-keyed registry ready for clp.EngineInput.Harmonised. When
// one entry lists multiple CAS numbers, the same harmonised limits are indexed
// under each number. Duplicate CAS entries are merged conservatively: the
// largest M-factor and lowest positive SCL for each H-code win.
func AsOf(date time.Time) (map[string]clp.Harmonised, error) {
	all, err := loadEntries()
	if err != nil {
		return nil, err
	}
	day := civilDate(date)
	registry := make(map[string]clp.Harmonised, len(all))
	for _, entry := range all {
		if !entryAppliesOn(entry, day) {
			continue
		}
		// Index under every identifier the entry carries. Several hundred Table 3
		// entries list an EC number and no CAS, so a CAS-only index leaves their
		// harmonised classification permanently unreachable.
		keys := append(append([]string(nil), entry.CASNumbers...), entry.ECNumbers...)
		for _, key := range keys {
			harmonised := registry[key]
			if entry.MFactorAcute > harmonised.MFactorAcute {
				harmonised.MFactorAcute = entry.MFactorAcute
			}
			if entry.MFactorChronic > harmonised.MFactorChronic {
				harmonised.MFactorChronic = entry.MFactorChronic
			}
			for _, limit := range entry.SCLs {
				harmonised.SCLs = mergeSCL(harmonised.SCLs, limit)
			}
			for _, hazard := range entry.Hazards {
				harmonised.Hazards = mergeHazard(harmonised.Hazards, hazard)
			}
			harmonised.Source = "annex_vi/" + DataVersion
			registry[key] = harmonised
		}
	}
	return registry, nil
}

// mergeHazard adds a harmonised hazard unless the same class is already present,
// so an identifier shared by several Table 3 entries carries each endpoint once.
func mergeHazard(existing []clp.HarmonisedHazard, candidate HarmonisedHazard) []clp.HarmonisedHazard {
	if strings.TrimSpace(candidate.Class) == "" {
		return existing
	}
	for _, hazard := range existing {
		if strings.EqualFold(hazard.Class, candidate.Class) {
			return existing
		}
	}
	return append(
		existing, clp.HarmonisedHazard{
			HCode:    candidate.HCode,
			Class:    candidate.Class,
			Category: candidate.Category,
		},
	)
}

// ATP23 returns the CAS registry at the ATP23 mandatory application date.
func ATP23() (map[string]clp.Harmonised, error) {
	date, _ := time.Parse(time.DateOnly, ATP23ApplicationDate)
	return AsOf(date)
}

func loadEntries() ([]Entry, error) {
	entryOnce.Do(
		func() {
			dataset, err := annexvidata.DecodeGZIP(bytes.NewReader(embeddedDataset))
			if err != nil {
				entriesErr = err
				return
			}
			if dataset.Version != DataVersion || dataset.Consolidation != ConsolidationCELEX {
				entriesErr = fmt.Errorf("Annex VI metadata mismatch: got %s/%s", dataset.Version, dataset.Consolidation)
				return
			}
			entries = make([]Entry, 0, len(dataset.Rows))
			for rowIndex, row := range dataset.Rows {
				entry, err := entryFromRow(row)
				if err != nil {
					entriesErr = fmt.Errorf("decode Annex VI row %d: %w", rowIndex, err)
					entries = nil
					return
				}
				entries = append(entries, entry)
			}
		},
	)
	return entries, entriesErr
}

func entryFromRow(row []string) (Entry, error) {
	record := Record{
		IndexNumber:                      row[0],
		ChemicalName:                     row[1],
		ECNumber:                         row[2],
		CASNumber:                        row[3],
		HazardClassCodes:                 row[4],
		HazardStatementCodes:             row[5],
		PictogramSignalWordCodes:         row[6],
		LabelHazardStatementCodes:        row[7],
		SupplementalHazardStatementCodes: row[8],
		SpecificLimits:                   row[9],
		Notes:                            row[10],
	}
	parsed := ParseRecord(record)
	from, err := time.Parse(time.DateOnly, row[11])
	if err != nil {
		return Entry{}, fmt.Errorf("parse applies_from %q: %w", row[11], err)
	}
	var until *time.Time
	if row[12] != "" {
		parsedUntil, err := time.Parse(time.DateOnly, row[12])
		if err != nil {
			return Entry{}, fmt.Errorf("parse applies_until %q: %w", row[12], err)
		}
		until = &parsedUntil
	}
	entry := Entry{
		IndexNumber:                      record.IndexNumber,
		ChemicalName:                     strings.Join(strings.Fields(record.ChemicalName), " "),
		ECNumbers:                        identifiers(record.ECNumber, nil),
		CASNumbers:                       identifiers(record.CASNumber, casPattern),
		HazardClassCodes:                 splitList(record.HazardClassCodes),
		HazardStatementCodes:             splitList(record.HazardStatementCodes),
		PictogramSignalWordCodes:         splitList(record.PictogramSignalWordCodes),
		LabelHazardStatementCodes:        splitList(record.LabelHazardStatementCodes),
		SupplementalHazardStatementCodes: splitList(record.SupplementalHazardStatementCodes),
		SpecificLimits:                   record.SpecificLimits,
		Notes:                            splitList(record.Notes),
		SCLs:                             parsed.SCLs,
		Hazards:                          parsed.Hazards,
		AppliesFrom:                      from,
		AppliesUntil:                     until,
		Source:                           row[13],
	}
	if parsed.MFactorAcute != nil {
		entry.MFactorAcute = *parsed.MFactorAcute
	}
	if parsed.MFactorChronic != nil {
		entry.MFactorChronic = *parsed.MFactorChronic
	}
	return entry, nil
}

func identifiers(value string, pattern *regexp.Regexp) []string {
	var result []string
	for _, identifier := range splitList(value) {
		identifier = cleanCAS(identifier)
		if identifier == "" || identifier == "-" || (pattern != nil && !pattern.MatchString(identifier)) {
			continue
		}
		result = append(result, identifier)
	}
	return result
}

func civilDate(date time.Time) time.Time {
	year, month, day := date.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func entryAppliesOn(entry Entry, day time.Time) bool {
	return !day.Before(entry.AppliesFrom) &&
		(entry.AppliesUntil == nil || day.Before(*entry.AppliesUntil))
}

func cloneEntry(entry Entry) Entry {
	entry.ECNumbers = append([]string(nil), entry.ECNumbers...)
	entry.CASNumbers = append([]string(nil), entry.CASNumbers...)
	entry.HazardClassCodes = append([]string(nil), entry.HazardClassCodes...)
	entry.HazardStatementCodes = append([]string(nil), entry.HazardStatementCodes...)
	entry.PictogramSignalWordCodes = append([]string(nil), entry.PictogramSignalWordCodes...)
	entry.LabelHazardStatementCodes = append([]string(nil), entry.LabelHazardStatementCodes...)
	entry.SupplementalHazardStatementCodes = append([]string(nil), entry.SupplementalHazardStatementCodes...)
	entry.Notes = append([]string(nil), entry.Notes...)
	entry.SCLs = append([]SCL(nil), entry.SCLs...)
	entry.Hazards = append([]HarmonisedHazard(nil), entry.Hazards...)
	if entry.AppliesUntil != nil {
		until := *entry.AppliesUntil
		entry.AppliesUntil = &until
	}
	return entry
}

func mergeSCL(existing []clp.HarmonisedSCL, candidate SCL) []clp.HarmonisedSCL {
	for index, limit := range existing {
		if strings.EqualFold(limit.HCode, candidate.HCode) {
			if candidate.Pct > 0 && (limit.Pct <= 0 || candidate.Pct < limit.Pct) {
				existing[index].Pct = candidate.Pct
			}
			return existing
		}
	}
	return append(existing, clp.HarmonisedSCL{HCode: candidate.HCode, Pct: candidate.Pct})
}
