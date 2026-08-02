package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cpouldev/go-clp/internal/annexvicheck"
)

func TestWriteReportAcceptsOrderOnlyDifferences(t *testing.T) {
	report := annexvicheck.Report{
		EURLEXCount:      4441,
		ECHACount:        4441,
		Matched:          4441,
		OrderDifferences: []annexvicheck.Difference{{IndexNumber: "001-001-00-9"}},
	}
	var output bytes.Buffer
	if err := writeReport(&output, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "order-only differences: 1") {
		t.Fatalf("report output = %q", output.String())
	}
}

func TestWriteReportRejectsSemanticOrMissingEntries(t *testing.T) {
	report := annexvicheck.Report{
		EURLEXCount:         4441,
		ECHACount:           4441,
		Matched:             4440,
		SemanticDifferences: []annexvicheck.Difference{{IndexNumber: "001-001-00-9"}},
		OnlyEURLEX:          []string{"002-002-00-8"},
	}
	if err := writeReport(&bytes.Buffer{}, report); err == nil {
		t.Fatal("writeReport accepted a substantive mismatch")
	}
}
