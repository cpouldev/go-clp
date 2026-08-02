package ufi

import "testing"

// TestGenerate_DistinctVATsDistinctUFIs guards the property the identifier
// exists for: two duty holders must never receive the same UFI for the same
// formulation. The Irish new-style VAT is the sharp case — it has a one-letter
// and a two-letter form, and treating the absent second letter as "A" issues one
// UFI to two companies.
func TestGenerate_DistinctVATsDistinctUFIs(t *testing.T) {
	cases := []struct{ country, vat string }{
		{"IE", "1234567A"},
		{"IE", "1234567AA"},
		{"IE", "1234567AB"},
		{"IE", "1234567W"},
		{"IE", "1234567WA"},
		{"IE", "1234567AW"},
		{"GR", "123456789"},
		{"GR", "123456780"},
		{"BE", "0123456789"},
		{"BE", "1123456789"},
		{"FR", "12345678901"},
	}

	seen := make(map[string]string, len(cases))
	for _, tc := range cases {
		got, err := Generate(tc.country, tc.vat, 17)
		if err != nil {
			t.Errorf("Generate(%q, %q) failed: %v", tc.country, tc.vat, err)
			continue
		}
		key := tc.country + " " + tc.vat
		if prev, clash := seen[got]; clash {
			t.Errorf("%s and %s both generate UFI %s", prev, key, got)
			continue
		}
		seen[got] = key

		if !Validate(got) {
			t.Errorf("Generate(%q, %q) = %s, which does not validate", tc.country, tc.vat, got)
		}
	}
}
