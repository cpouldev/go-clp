package allergens

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
)

const (
	// ConsolidationCELEX identifies the immutable EUR-Lex source used for the
	// embedded registry.
	ConsolidationCELEX = "02009R1223-20260501"

	// LeaveOnThresholdPercent is the concentration above which an allergen must
	// be named in a leave-on cosmetic product's ingredient list.
	LeaveOnThresholdPercent = 0.001

	// RinseOffThresholdPercent is the concentration above which an allergen must
	// be named in a rinse-off cosmetic product's ingredient list.
	RinseOffThresholdPercent = 0.01
)

// ProductKind selects the Annex III individual-labelling threshold.
type ProductKind string

const (
	// LeaveOn selects the 0.001% threshold.
	LeaveOn ProductKind = "leave-on"
	// RinseOff selects the 0.01% threshold.
	RinseOff ProductKind = "rinse-off"
)

// Entry is one Annex III reference subject to individual fragrance-allergen
// labelling. One reference may cover several substances and synonyms.
type Entry struct {
	ReferenceNumber string
	ChemicalNames   []string
	INCINames       []string
	CASNumbers      []string
	ECNumbers       []string
	INCIByCAS       map[string]string
}

// Registry contains fresh lookup maps suitable for EngineInput.AllergenCAS and
// EngineInput.InciNames. Callers may modify the returned data.
type Registry struct {
	Entries   []Entry
	CAS       map[string]bool
	INCIByCAS map[string]string
}

type encodedDataset struct {
	CELEX   string         `json:"celex"`
	Entries []encodedEntry `json:"entries"`
}

type encodedEntry struct {
	ReferenceNumber string            `json:"referenceNumber"`
	ChemicalNames   []string          `json:"chemicalNames,omitempty"`
	INCINames       []string          `json:"inciNames,omitempty"`
	CASNumbers      []string          `json:"casNumbers,omitempty"`
	ECNumbers       []string          `json:"ecNumbers,omitempty"`
	INCIByCAS       map[string]string `json:"inciByCas,omitempty"`
}

//go:generate go run ../cmd/allergens-gen -out data/allergens.json.gz

//go:embed data/allergens.json.gz
var compressedDataset []byte

var (
	loadOnce sync.Once
	loaded   Registry
	loadErr  error
)

// Load returns the embedded Annex III fragrance-allergen registry. Each call
// returns independent slices and maps, safe for caller modification.
func Load() (Registry, error) {
	loadOnce.Do(load)
	if loadErr != nil {
		return Registry{}, loadErr
	}
	return cloneRegistry(loaded), nil
}

// RequiresDisclosure reports whether concentrationPercent exceeds the Annex
// III individual-labelling threshold for kind. The threshold is strict: a value
// equal to the threshold does not exceed it.
func RequiresDisclosure(kind ProductKind, concentrationPercent float64) (bool, error) {
	if concentrationPercent < 0 || concentrationPercent > 100 || math.IsNaN(concentrationPercent) || math.IsInf(concentrationPercent, 0) {
		return false, fmt.Errorf("allergens: concentration must be a finite percentage from 0 to 100")
	}
	var threshold float64
	switch kind {
	case LeaveOn:
		threshold = LeaveOnThresholdPercent
	case RinseOff:
		threshold = RinseOffThresholdPercent
	default:
		return false, fmt.Errorf("allergens: unknown product kind %q", kind)
	}
	return concentrationPercent > threshold, nil
}

func load() {
	reader, err := gzip.NewReader(bytes.NewReader(compressedDataset))
	if err != nil {
		loadErr = fmt.Errorf("allergens: open embedded dataset: %w", err)
		return
	}
	defer reader.Close()

	var dataset encodedDataset
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&dataset); err != nil {
		loadErr = fmt.Errorf("allergens: decode embedded dataset: %w", err)
		return
	}
	if err := ensureEOF(decoder); err != nil {
		loadErr = err
		return
	}
	if dataset.CELEX != ConsolidationCELEX {
		loadErr = fmt.Errorf("allergens: embedded CELEX is %q, want %q", dataset.CELEX, ConsolidationCELEX)
		return
	}

	registry := Registry{
		Entries:   make([]Entry, 0, len(dataset.Entries)),
		CAS:       make(map[string]bool),
		INCIByCAS: make(map[string]string),
	}
	for _, encoded := range dataset.Entries {
		entry := Entry{
			ReferenceNumber: encoded.ReferenceNumber,
			ChemicalNames:   append([]string(nil), encoded.ChemicalNames...),
			INCINames:       append([]string(nil), encoded.INCINames...),
			CASNumbers:      append([]string(nil), encoded.CASNumbers...),
			ECNumbers:       append([]string(nil), encoded.ECNumbers...),
			INCIByCAS:       trimmedStringMap(encoded.INCIByCAS),
		}
		registry.Entries = append(registry.Entries, entry)
		for _, cas := range entry.CASNumbers {
			registry.CAS[cas] = true
		}
		for cas, name := range entry.INCIByCAS {
			if _, exists := registry.INCIByCAS[cas]; !exists {
				registry.INCIByCAS[cas] = name
			}
		}
	}
	inciNames := 0
	for _, entry := range registry.Entries {
		inciNames += len(entry.INCINames)
	}
	if len(registry.Entries) != 81 || len(registry.CAS) != 167 || inciNames != 140 {
		loadErr = fmt.Errorf("allergens: invalid embedded dataset counts: %d entries, %d CAS numbers, %d distinct INCI names", len(registry.Entries), len(registry.CAS), inciNames)
		return
	}
	loaded = registry
}

// trimmedStringMap clones a CAS-to-INCI map with both sides trimmed. The source
// table carries a leading space on a handful of INCI cells, which would
// otherwise make a lookup through this map disagree with the same name read from
// Entry.INCINames.
func trimmedStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return clone
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("allergens: decode embedded dataset trailer: %w", err)
	}
	return fmt.Errorf("allergens: embedded dataset contains multiple JSON values")
}

func cloneRegistry(source Registry) Registry {
	clone := Registry{
		Entries:   make([]Entry, len(source.Entries)),
		CAS:       make(map[string]bool, len(source.CAS)),
		INCIByCAS: cloneStringMap(source.INCIByCAS),
	}
	for i, entry := range source.Entries {
		clone.Entries[i] = Entry{
			ReferenceNumber: entry.ReferenceNumber,
			ChemicalNames:   append([]string(nil), entry.ChemicalNames...),
			INCINames:       append([]string(nil), entry.INCINames...),
			CASNumbers:      append([]string(nil), entry.CASNumbers...),
			ECNumbers:       append([]string(nil), entry.ECNumbers...),
			INCIByCAS:       cloneStringMap(entry.INCIByCAS),
		}
	}
	for cas, present := range source.CAS {
		clone.CAS[cas] = present
	}
	return clone
}

func cloneStringMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
