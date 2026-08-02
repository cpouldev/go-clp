package ufi

import (
	"strings"
	"testing"
)

func TestGenerateMatchesECHAOfficialGenerator(t *testing.T) {
	tests := []struct {
		name        string
		country     string
		vat         string
		formulation int
		want        string
	}{
		{"Greece minimum", "GR", "156329319", 0, "J000-808J-8002-UH2J"},
		{"Greece one", "GR", "156329319", 1, "1300-R0XX-J00J-GUNM"},
		{"Greece maximum", "GR", "156329319", MaxFormulationNumber, "D3NN-6KD9-PXSY-WCR0"},
		{"Austria", "AT", "U12345678", 178956970, "C23S-PQ2V-AMH9-VVRF"},
		{"France", "FR", "AB123456789", 123456, "27W0-KC81-E00T-V314"},
		{"United Kingdom", "GB", "123456789", 42, "WM30-40MR-N00G-6N25"},
		{"Netherlands", "NL", "123456789B01", 999, "0SR2-70TN-G00G-HE4Q"},
		{"Ireland", "IE", "1234567AW", 17, "6G10-G0J1-N001-2QCV"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Generate(tt.country, tt.vat, tt.formulation)
			if err != nil {
				t.Fatalf("Generate(%q, %q, %d): %v", tt.country, tt.vat, tt.formulation, err)
			}
			if got != tt.want {
				t.Fatalf("Generate(%q, %q, %d) = %q, want %q", tt.country, tt.vat, tt.formulation, got, tt.want)
			}
			if !Validate(got) {
				t.Fatalf("Validate(%q) = false for generated UFI", got)
			}
		})
	}
}

func TestGenerateSupportsEveryECHACountry(t *testing.T) {
	tests := []struct {
		country string
		vat     string
	}{
		{"EU", "1000000746912"},
		{"FR", "AB123456789"},
		{"GB", "123456789"},
		{"LT", "123456789"},
		{"SE", "123456789012"},
		{"HR", "12345678901"},
		{"IT", "12345678901"},
		{"LV", "12345678901"},
		{"NL", "123456789B01"},
		{"BG", "123456789"},
		{"CZ", "12345678"},
		{"IE", "1234567AW"},
		{"ES", "A1234567B"},
		{"PL", "1234567890"},
		{"RO", "12"},
		{"SK", "1234567890"},
		{"CY", "12345678A"},
		{"IS", "ABC123"},
		{"BE", "0123456789"},
		{"DE", "123456789"},
		{"EE", "123456789"},
		{"GR", "156329319"},
		{"NO", "123456789"},
		{"PT", "123456789"},
		{"AT", "U12345678"},
		{"DK", "12345678"},
		{"FI", "12345678"},
		{"HU", "12345678"},
		{"LU", "12345678"},
		{"MT", "12345678"},
		{"SI", "12345678"},
		{"LI", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.country, func(t *testing.T) {
			got, err := Generate(tt.country, tt.vat, 123)
			if err != nil {
				t.Fatalf("Generate(%q, %q, 123): %v", tt.country, tt.vat, err)
			}
			if !Validate(got) {
				t.Fatalf("generated invalid UFI %q", got)
			}
		})
	}
}

func TestGenerateNormalizesCountryAndPrefix(t *testing.T) {
	want, err := Generate("GR", "156329319", 42)
	if err != nil {
		t.Fatal(err)
	}

	for _, input := range []struct {
		country string
		vat     string
	}{
		{"EL", "EL156329319"},
		{"gr", "GR156329319"},
		{" GR ", " 156329319 "},
	} {
		got, err := Generate(input.country, input.vat, 42)
		if err != nil {
			t.Errorf("Generate(%q, %q, 42): %v", input.country, input.vat, err)
			continue
		}
		if got != want {
			t.Errorf("Generate(%q, %q, 42) = %q, want %q", input.country, input.vat, got, want)
		}
	}
}

func TestGenerateRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name        string
		country     string
		vat         string
		formulation int
	}{
		{"unknown country", "ZZ", "123456789", 1},
		{"invalid Austrian VAT", "AT", "12345678", 1},
		{"invalid Dutch VAT", "NL", "123456789X01", 1},
		{"empty VAT", "DE", "", 1},
		{"negative formulation", "DE", "123456789", -1},
		{"formulation too large", "DE", "123456789", MaxFormulationNumber + 1},
		{"EU key exceeds payload", "EU", "9999999999999", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Generate(tt.country, tt.vat, tt.formulation)
			if err == nil {
				t.Fatalf("Generate(%q, %q, %d) = %q, want error", tt.country, tt.vat, tt.formulation, got)
			}
			if got != "" {
				t.Fatalf("Generate returned %q with error, want empty string", got)
			}
		})
	}
}

func TestValidateRejectsCorruptionAndBadShape(t *testing.T) {
	valid, err := Generate("AT", "U12345678", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(valid) {
		t.Fatalf("Validate(%q) = false", valid)
	}

	for i := range valid {
		if valid[i] == '-' {
			continue
		}
		replacement := byte('0')
		if valid[i] == replacement {
			replacement = '1'
		}
		corrupt := valid[:i] + string(replacement) + valid[i+1:]
		if Validate(corrupt) {
			t.Errorf("Validate(%q) = true after corruption at byte %d", corrupt, i)
		}
	}

	for _, bad := range []string{"", "B000-808J-8002-UH2J", "J000-808J-8002-UH2", strings.ToLower(valid)} {
		if Validate(bad) {
			t.Errorf("Validate(%q) = true, want false", bad)
		}
	}
}

func FuzzGenerateRoundTrip(f *testing.F) {
	for _, n := range []int{0, 1, 31, 1 << 24, MaxFormulationNumber} {
		f.Add(n)
	}
	f.Fuzz(func(t *testing.T, formulation int) {
		if formulation < 0 || formulation > MaxFormulationNumber {
			t.Skip()
		}
		first, err := Generate("DE", "123456789", formulation)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Generate("DE", "123456789", formulation)
		if err != nil {
			t.Fatal(err)
		}
		if first != second {
			t.Fatalf("Generate is not deterministic: %q then %q", first, second)
		}
		if !Validate(first) {
			t.Fatalf("Validate(%q) = false", first)
		}
	})
}
