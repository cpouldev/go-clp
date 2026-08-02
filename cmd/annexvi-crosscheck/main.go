// Command annexvi-crosscheck compares a manually supplied ECHA Annex VI CSV
// with the embedded EUR-Lex ATP23 snapshot. It performs no network access and
// writes no data artifacts.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cpouldev/go-clp/annexvi"
	"github.com/cpouldev/go-clp/internal/annexvicheck"
)

func main() {
	var echaPath string
	flag.StringVar(&echaPath, "echa", "", "path to a manually exported ECHA Annex VI CSV")
	flag.Parse()
	if echaPath == "" {
		fmt.Fprintln(os.Stderr, "annexvi-crosscheck: -echa is required")
		os.Exit(2)
	}
	if err := run(echaPath, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(echaPath string, output io.Writer) error {
	file, err := os.Open(echaPath)
	if err != nil {
		return fmt.Errorf("annexvi-crosscheck: open ECHA CSV: %w", err)
	}
	defer file.Close()

	echa, err := annexvicheck.ReadCSV(file)
	if err != nil {
		return err
	}
	date, err := time.Parse("2006-01-02", annexvi.ATP23ApplicationDate)
	if err != nil {
		return fmt.Errorf("annexvi-crosscheck: parse ATP23 date: %w", err)
	}
	eurlex, err := annexvi.EntriesAsOf(date)
	if err != nil {
		return fmt.Errorf("annexvi-crosscheck: load EUR-Lex registry: %w", err)
	}
	return writeReport(output, annexvicheck.Compare(eurlex, echa))
}

func writeReport(output io.Writer, report annexvicheck.Report) error {
	fmt.Fprintf(output, "EUR-Lex entries: %d\n", report.EURLEXCount)
	fmt.Fprintf(output, "ECHA entries: %d\n", report.ECHACount)
	fmt.Fprintf(output, "matched index numbers: %d\n", report.Matched)
	fmt.Fprintf(output, "order-only differences: %d\n", len(report.OrderDifferences))
	fmt.Fprintf(output, "semantic differences: %d\n", len(report.SemanticDifferences))
	fmt.Fprintf(output, "only in EUR-Lex: %d\n", len(report.OnlyEURLEX))
	fmt.Fprintf(output, "only in ECHA: %d\n", len(report.OnlyECHA))

	if len(report.SemanticDifferences) == 0 && len(report.OnlyEURLEX) == 0 && len(report.OnlyECHA) == 0 {
		return nil
	}
	if len(report.SemanticDifferences) > 0 {
		indexes := make([]string, len(report.SemanticDifferences))
		for i, difference := range report.SemanticDifferences {
			indexes[i] = difference.IndexNumber
		}
		fmt.Fprintf(output, "semantic mismatch indexes: %s\n", strings.Join(indexes, ", "))
	}
	if len(report.OnlyEURLEX) > 0 {
		fmt.Fprintf(output, "EUR-Lex-only indexes: %s\n", strings.Join(report.OnlyEURLEX, ", "))
	}
	if len(report.OnlyECHA) > 0 {
		fmt.Fprintf(output, "ECHA-only indexes: %s\n", strings.Join(report.OnlyECHA, ", "))
	}
	return fmt.Errorf("annexvi-crosscheck: substantive source differences found")
}
