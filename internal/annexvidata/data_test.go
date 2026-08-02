package annexvidata

import (
	"bytes"
	"reflect"
	"testing"
)

func TestGZIPRoundTrip(t *testing.T) {
	want := Dataset{
		Version:       "ATP23",
		Consolidation: "02008R1272-20260701",
		Sources:       []string{"02008R1272-20260701", "32025R1222"},
		Columns:       Columns,
		Rows: [][]string{{
			"001-001-00-9", "hydrogen", "215-605-7", "1333-74-0",
			"Flam. Gas 1", "H220", "GHS02", "H220", "", "", "",
			"2026-05-01", "", "02008R1272-20260701",
		}},
	}

	var first bytes.Buffer
	if err := EncodeGZIP(&first, want); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeGZIP(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}

	var second bytes.Buffer
	if err := EncodeGZIP(&second, want); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("EncodeGZIP is not reproducible")
	}
}

func TestDecodeRejectsWrongColumns(t *testing.T) {
	var encoded bytes.Buffer
	if err := EncodeGZIP(&encoded, Dataset{Columns: []string{"wrong"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeGZIP(bytes.NewReader(encoded.Bytes())); err == nil {
		t.Fatal("DecodeGZIP accepted an incompatible schema")
	}
}
