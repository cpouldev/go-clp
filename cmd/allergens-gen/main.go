// Command allergens-gen regenerates the embedded Cosmetics Annex III
// fragrance-allergen registry from a pinned EUR-Lex consolidation.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cpouldev/go-clp/internal/cosmeticssource"
)

const (
	defaultCELEX = "02009R1223-20260501"
	defaultURL   = "https://publications.europa.eu/resource/celex/" + defaultCELEX
	defaultOut   = "allergens/data/allergens.json.gz"
	entryCount   = 81
	casCount     = 167
	inciCount    = 140
)

type dataset struct {
	CELEX   string                   `json:"celex"`
	Entries []cosmeticssource.Record `json:"entries"`
}

func main() {
	var source string
	var output string
	flag.StringVar(&source, "source", "", "local EUR-Lex XHTML file (downloads the pinned source when empty)")
	flag.StringVar(&output, "out", defaultOut, "output gzip file")
	flag.Parse()

	if err := run(source, output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(source, output string) error {
	reader, err := sourceReader(source)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	records, err := cosmeticssource.ParseXHTML(reader)
	if err != nil {
		return err
	}
	if len(records) != entryCount {
		return fmt.Errorf("allergens-gen: parsed %d entries, want %d", len(records), entryCount)
	}
	cas := make(map[string]bool)
	inciNames := 0
	for _, record := range records {
		inciNames += len(record.INCINames)
		for _, number := range record.CASNumbers {
			cas[number] = true
		}
	}
	if len(cas) != casCount {
		return fmt.Errorf("allergens-gen: parsed %d distinct CAS numbers, want %d", len(cas), casCount)
	}
	if inciNames != inciCount {
		return fmt.Errorf("allergens-gen: parsed %d distinct INCI names, want %d", inciNames, inciCount)
	}

	return writeDataset(output, dataset{CELEX: defaultCELEX, Entries: records})
}

func sourceReader(path string) (io.ReadCloser, error) {
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("allergens-gen: open source: %w", err)
		}
		return file, nil
	}

	request, err := http.NewRequest(http.MethodGet, defaultURL, nil)
	if err != nil {
		return nil, fmt.Errorf("allergens-gen: create request: %w", err)
	}
	request.Header.Set("Accept", "application/xhtml+xml")
	request.Header.Set("Accept-Language", "eng")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("allergens-gen: download source: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("allergens-gen: download source: HTTP %s", response.Status)
	}
	return response.Body, nil
}

func writeDataset(path string, value dataset) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("allergens-gen: create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".allergens-*.gz")
	if err != nil {
		return fmt.Errorf("allergens-gen: create temporary output: %w", err)
	}
	temporaryName := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryName)
		}
	}()

	zipper, err := gzip.NewWriterLevel(temporary, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("allergens-gen: create gzip writer: %w", err)
	}
	zipper.ModTime = gzip.Header{}.ModTime
	zipper.OS = 255
	encoder := json.NewEncoder(zipper)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("allergens-gen: encode dataset: %w", err)
	}
	if err := zipper.Close(); err != nil {
		return fmt.Errorf("allergens-gen: close gzip output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("allergens-gen: set output permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("allergens-gen: close output: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("allergens-gen: replace output: %w", err)
	}
	keep = true
	return nil
}
