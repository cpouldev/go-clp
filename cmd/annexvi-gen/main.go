// Command annexvi-gen regenerates the embedded CLP Annex VI registry from
// immutable EUR-Lex sources.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cpouldev/go-clp/internal/annexvidata"
	"github.com/cpouldev/go-clp/internal/annexvisource"
)

const (
	dataVersion = "ATP23"
	baseCELEX   = "02008R1272-20260701"
	atp23CELEX  = "32025R1222"

	baseURL  = "https://publications.europa.eu/resource/celex/" + baseCELEX
	atp23URL = "https://publications.europa.eu/resource/celex/" + atp23CELEX

	baseApplicationDate  = "2026-05-01"
	atp23ApplicationDate = "2027-02-01"

	expectedBaseRecords  = 4419
	expectedATP23Records = 32
)

func main() {
	baseInput := flag.String("base", baseURL, "pinned consolidated EUR-Lex XHTML URL or local file")
	atp23Input := flag.String("atp23", atp23URL, "ATP23 EUR-Lex XHTML URL or local file")
	output := flag.String("out", "annexvi/data/annexvi.json.gz", "output gzip path")
	flag.Parse()

	if err := run(context.Background(), *baseInput, *atp23Input, *output); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, baseInput, atp23Input, output string) error {
	base, err := readRecords(ctx, baseInput)
	if err != nil {
		return fmt.Errorf("read consolidated Annex VI: %w", err)
	}
	changes, err := readRecords(ctx, atp23Input)
	if err != nil {
		return fmt.Errorf("read ATP23 amendment: %w", err)
	}
	if err := validateSourceCounts(base, changes); err != nil {
		return err
	}
	dataset, err := buildDataset(base, changes)
	if err != nil {
		return err
	}
	if err := writeDataset(output, dataset); err != nil {
		return err
	}
	info, err := os.Stat(output)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %d versioned rows (%d bytes) to %s\n", len(dataset.Rows), info.Size(), output)
	return nil
}

func validateSourceCounts(base, changes []annexvisource.Record) error {
	if len(base) != expectedBaseRecords {
		return fmt.Errorf("consolidated Annex VI has %d records, want pinned count %d", len(base), expectedBaseRecords)
	}
	if len(changes) != expectedATP23Records {
		return fmt.Errorf("ATP23 amendment has %d changed records, want pinned count %d", len(changes), expectedATP23Records)
	}
	return nil
}

func readRecords(ctx context.Context, input string) ([]annexvisource.Record, error) {
	reader, err := openInput(ctx, input)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return annexvisource.ParseXHTML(reader)
}

func openInput(ctx context.Context, input string) (io.ReadCloser, error) {
	if !strings.HasPrefix(input, "http://") && !strings.HasPrefix(input, "https://") {
		return os.Open(input)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/xhtml+xml")
	request.Header.Set("Accept-Language", "eng")
	request.Header.Set("User-Agent", "go-clp-annexvi-gen/1")
	client := &http.Client{Timeout: 3 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", input, response.Status)
	}
	return response.Body, nil
}

func buildDataset(base, changes []annexvisource.Record) (annexvidata.Dataset, error) {
	baseDate, _ := time.Parse(time.DateOnly, baseApplicationDate)
	atp23Date, _ := time.Parse(time.DateOnly, atp23ApplicationDate)
	baseVersions := make([]annexvisource.VersionedRecord, len(base))
	for i, record := range base {
		baseVersions[i] = annexvisource.VersionedRecord{Record: record}
	}
	changeVersions := make([]annexvisource.VersionedRecord, len(changes))
	for i, record := range changes {
		changeVersions[i] = annexvisource.VersionedRecord{Record: record}
	}
	versions, err := annexvisource.MergeVersions(baseVersions, changeVersions, baseDate, atp23Date)
	if err != nil {
		return annexvidata.Dataset{}, err
	}

	rows := make([][]string, 0, len(versions))
	for _, version := range versions {
		source := baseCELEX
		if !version.AppliesFrom.Before(atp23Date) {
			source = atp23CELEX
		}
		until := ""
		if version.AppliesUntil != nil {
			until = version.AppliesUntil.Format(time.DateOnly)
		}
		rows = append(rows, []string{
			version.IndexNumber,
			version.ChemicalName,
			version.ECNumber,
			version.CASNumber,
			version.HazardClassCodes,
			version.HazardStatementCodes,
			version.PictogramSignalWordCodes,
			version.LabelHazardStatementCodes,
			version.SupplementalHazardStatementCodes,
			version.SpecificLimits,
			version.Notes,
			version.AppliesFrom.Format(time.DateOnly),
			until,
			source,
		})
	}
	return annexvidata.Dataset{
		Version:       dataVersion,
		Consolidation: baseCELEX,
		Sources:       []string{baseCELEX, atp23CELEX},
		Columns:       append([]string(nil), annexvidata.Columns...),
		Rows:          rows,
	}, nil
}

func writeDataset(path string, dataset annexvidata.Dataset) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".annexvi-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := annexvidata.EncodeGZIP(temporary, dataset); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}
