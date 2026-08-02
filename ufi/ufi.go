// Package ufi generates and validates Unique Formula Identifiers according to
// the ECHA UFI Developers Manual.
package ufi

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	// MaxFormulationNumber is the largest value that fits the UFI payload's
	// 28-bit formulation-number field.
	MaxFormulationNumber = 1<<28 - 1

	base31Alphabet = "0123456789ACDEFGHJKMNPQRSTUVWXY"
	ufiDigits      = 15
)

var digitOrder = [ufiDigits]int{5, 4, 3, 7, 2, 8, 9, 10, 1, 0, 11, 6, 12, 13, 14}

type countrySpec struct {
	group    uint64
	code     uint64
	codeBits uint
}

var countrySpecs = map[string]countrySpec{
	"EU": {group: 0},
	"FR": {group: 1},
	"GB": {group: 2},
	"LT": {group: 3, code: 0, codeBits: 1},
	"SE": {group: 3, code: 1, codeBits: 1},
	"HR": {group: 4, code: 0, codeBits: 4},
	"IT": {group: 4, code: 1, codeBits: 4},
	"LV": {group: 4, code: 2, codeBits: 4},
	"NL": {group: 4, code: 3, codeBits: 4},
	"BG": {group: 5, code: 0, codeBits: 7},
	"CZ": {group: 5, code: 1, codeBits: 7},
	"IE": {group: 5, code: 2, codeBits: 7},
	"ES": {group: 5, code: 3, codeBits: 7},
	"PL": {group: 5, code: 4, codeBits: 7},
	"RO": {group: 5, code: 5, codeBits: 7},
	"SK": {group: 5, code: 6, codeBits: 7},
	"CY": {group: 5, code: 7, codeBits: 7},
	"IS": {group: 5, code: 8, codeBits: 7},
	"BE": {group: 5, code: 9, codeBits: 7},
	"DE": {group: 5, code: 10, codeBits: 7},
	"EE": {group: 5, code: 11, codeBits: 7},
	"GR": {group: 5, code: 12, codeBits: 7},
	"NO": {group: 5, code: 13, codeBits: 7},
	"PT": {group: 5, code: 14, codeBits: 7},
	"AT": {group: 5, code: 15, codeBits: 7},
	"DK": {group: 5, code: 16, codeBits: 7},
	"FI": {group: 5, code: 17, codeBits: 7},
	"HU": {group: 5, code: 18, codeBits: 7},
	"LU": {group: 5, code: 19, codeBits: 7},
	"MT": {group: 5, code: 20, codeBits: 7},
	"SI": {group: 5, code: 21, codeBits: 7},
	"LI": {group: 5, code: 22, codeBits: 7},
}

// Generate returns the displayed XXXX-XXXX-XXXX-XXXX UFI for a country, VAT
// number (or EU company key), and formulation number. Country accepts EL as an
// alias for GR and XN as an alias for GB. VAT prefixes and the separators that
// ECHA's web form ignores may be included.
func Generate(country, vat string, formulationNumber int) (string, error) {
	if formulationNumber < 0 || formulationNumber > MaxFormulationNumber {
		return "", fmt.Errorf("ufi: formulation number %d out of range [0, %d]", formulationNumber, MaxFormulationNumber)
	}

	canonical := normalizeCountry(country)
	spec, ok := countrySpecs[canonical]
	if !ok {
		return "", fmt.Errorf("ufi: unsupported country code %q", country)
	}

	vatValue, err := parseVAT(canonical, vat)
	if err != nil {
		return "", err
	}
	vatBits := uint(41) - spec.codeBits
	if vatValue >= uint64(1)<<vatBits {
		return "", fmt.Errorf("ufi: VAT value for country %s does not fit in %d bits", canonical, vatBits)
	}

	payload := buildPayload(uint64(formulationNumber), spec, vatValue)
	digits := payloadToBase31(payload)
	reorganized := reorganize(digits)
	return format(checksum(reorganized), reorganized), nil
}

func normalizeCountry(country string) string {
	country = strings.ToUpper(strings.TrimSpace(country))
	switch country {
	case "EL":
		return "GR"
	case "XN":
		return "GB"
	default:
		return country
	}
}

func parseVAT(country, input string) (uint64, error) {
	vat := strings.ToUpper(strings.TrimSpace(input))
	vat = strings.NewReplacer(" ", "", ".", "", "-", "").Replace(vat)

	if value, ok := parseBareVAT(country, vat); ok {
		return value, nil
	}
	for _, prefix := range vatPrefixes(country) {
		if strings.HasPrefix(vat, prefix) {
			if value, ok := parseBareVAT(country, strings.TrimPrefix(vat, prefix)); ok {
				return value, nil
			}
		}
	}
	return 0, fmt.Errorf("ufi: VAT %q is invalid for country %s", input, country)
}

func vatPrefixes(country string) []string {
	switch country {
	case "GR":
		return []string{"EL", "GR"}
	case "GB":
		return []string{"GB", "XN"}
	default:
		return []string{country}
	}
}

func parseBareVAT(country, vat string) (uint64, bool) {
	switch country {
	case "EU":
		return parseDigits(vat, 13, 13)
	case "AT":
		if len(vat) != 9 || vat[0] != 'U' {
			return 0, false
		}
		return parseDigits(vat[1:], 8, 8)
	case "BE":
		return parseDigits(vat, 10, 10)
	case "BG":
		return parseDigits(vat, 9, 10)
	case "CY":
		if len(vat) != 9 || !isLetter(vat[8]) {
			return 0, false
		}
		digits, ok := parseDigits(vat[:8], 8, 8)
		return uint64(vat[8]-'A')*100_000_000 + digits, ok
	case "CZ":
		return parseDigits(vat, 8, 10)
	case "DE", "EE", "GR", "NO", "PT":
		return parseDigits(vat, 9, 9)
	case "DK", "FI", "HU", "LU", "MT", "SI":
		return parseDigits(vat, 8, 8)
	case "ES":
		return parseSpanishVAT(vat)
	case "FR":
		return parseFrenchVAT(vat)
	case "GB":
		return parseBritishVAT(vat)
	case "IE":
		return parseIrishVAT(vat)
	case "IS":
		return parseIcelandicVAT(vat)
	case "HR", "IT", "LV":
		return parseDigits(vat, 11, 11)
	case "LI":
		return parseDigits(vat, 5, 5)
	case "LT":
		if len(vat) != 9 && len(vat) != 12 {
			return 0, false
		}
		return parseDigits(vat, len(vat), len(vat))
	case "NL":
		if len(vat) != 12 || vat[9] != 'B' {
			return 0, false
		}
		return parseDigits(vat[:9]+vat[10:], 11, 11)
	case "PL", "SK":
		return parseDigits(vat, 10, 10)
	case "RO":
		return parseDigits(vat, 2, 10)
	case "SE":
		return parseDigits(vat, 12, 12)
	default:
		return 0, false
	}
}

func parseSpanishVAT(vat string) (uint64, bool) {
	if len(vat) != 9 || !isBase36(vat[0]) || !isBase36(vat[8]) {
		return 0, false
	}
	digits, ok := parseDigits(vat[1:8], 7, 7)
	if !ok {
		return 0, false
	}
	return uint64(36*base36Value(vat[0])+base36Value(vat[8]))*10_000_000 + digits, true
}

func parseFrenchVAT(vat string) (uint64, bool) {
	if len(vat) != 11 || !isBase36(vat[0]) || !isBase36(vat[1]) {
		return 0, false
	}
	digits, ok := parseDigits(vat[2:], 9, 9)
	if !ok {
		return 0, false
	}
	return uint64(36*base36Value(vat[0])+base36Value(vat[1]))*1_000_000_000 + digits, true
}

func parseBritishVAT(vat string) (uint64, bool) {
	if len(vat) == 9 || len(vat) == 12 {
		digits, ok := parseDigits(vat, len(vat), len(vat))
		if ok {
			return uint64(1)<<40 + digits, true
		}
	}
	if len(vat) != 5 || !isLetter(vat[0]) || !isLetter(vat[1]) {
		return 0, false
	}
	digits, ok := parseDigits(vat[2:], 3, 3)
	if !ok {
		return 0, false
	}
	return uint64(26*int(vat[0]-'A')+int(vat[1]-'A'))*1_000 + digits, true
}

func parseIrishVAT(vat string) (uint64, bool) {
	if len(vat) == 8 && isDigit(vat[0]) && isIrishSecond(vat[1]) && allDigits(vat[2:7]) && isLetter(vat[7]) {
		first := int(vat[1] - 'A')
		switch vat[1] {
		case '+':
			first = 26
		case '*':
			first = 27
		}
		digits, _ := strconv.ParseUint(vat[:1]+vat[2:7], 10, 64)
		return uint64(26*first+int(vat[7]-'A'))*1_000_000 + digits, true
	}
	if (len(vat) == 8 || len(vat) == 9) && allDigits(vat[:7]) && isLetter(vat[7]) && (len(vat) == 8 || isLetter(vat[8])) {
		digits, _ := strconv.ParseUint(vat[:7], 10, 64)
		first := uint64(vat[7] - 'A')

		// An ABSENT second letter is not the same duty holder as a second letter
		// "A": leaving it at zero encodes "1234567A" and "1234567AA" identically
		// and issues one UFI to two companies. 26 is the first value outside the
		// A-Z range, so every two-letter VAT keeps the encoding it already had.
		second := uint64(26)
		if len(vat) == 9 {
			second = uint64(vat[8] - 'A')
		}
		return uint64(1)<<33 + (26*second+first)*10_000_000 + digits, true
	}
	return 0, false
}

func parseIcelandicVAT(vat string) (uint64, bool) {
	if len(vat) != 6 {
		return 0, false
	}
	var value uint64
	for i := range vat {
		if !isBase36(vat[i]) {
			return 0, false
		}
		value = value*36 + uint64(base36Value(vat[i]))
	}
	return value, true
}

func parseDigits(value string, minLength, maxLength int) (uint64, bool) {
	if len(value) < minLength || len(value) > maxLength || !allDigits(value) {
		return 0, false
	}
	n, err := strconv.ParseUint(value, 10, 64)
	return n, err == nil
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := range value {
		if !isDigit(value[i]) {
			return false
		}
	}
	return true
}

func isDigit(char byte) bool  { return char >= '0' && char <= '9' }
func isLetter(char byte) bool { return char >= 'A' && char <= 'Z' }
func isBase36(char byte) bool { return isDigit(char) || isLetter(char) }
func isIrishSecond(char byte) bool {
	return isLetter(char) || char == '+' || char == '*'
}

func base36Value(char byte) int {
	if isDigit(char) {
		return int(char - '0')
	}
	return int(char-'A') + 10
}

func buildPayload(formulationNumber uint64, spec countrySpec, vat uint64) *big.Int {
	payload := new(big.Int).SetUint64(formulationNumber)
	payload.Lsh(payload, 4)
	payload.Or(payload, new(big.Int).SetUint64(spec.group))
	if spec.codeBits > 0 {
		payload.Lsh(payload, spec.codeBits)
		payload.Or(payload, new(big.Int).SetUint64(spec.code))
	}
	payload.Lsh(payload, uint(42)-spec.codeBits)
	payload.Or(payload, new(big.Int).SetUint64(vat<<1))
	return payload
}

func payloadToBase31(payload *big.Int) [ufiDigits]int {
	var digits [ufiDigits]int
	work := new(big.Int).Set(payload)
	base := big.NewInt(31)
	mod := new(big.Int)
	for i := ufiDigits - 1; i >= 0; i-- {
		work.QuoRem(work, base, mod)
		digits[i] = int(mod.Int64())
	}
	return digits
}

func reorganize(digits [ufiDigits]int) [ufiDigits]int {
	var result [ufiDigits]int
	for i, source := range digitOrder {
		result[i] = digits[source]
	}
	return result
}

func checksum(digits [ufiDigits]int) byte {
	sum := 0
	for i, digit := range digits {
		sum += (i + 2) * digit
	}
	return base31Alphabet[(31-sum%31)%31]
}

func format(checksum byte, digits [ufiDigits]int) string {
	var builder strings.Builder
	builder.Grow(19)
	builder.WriteByte(checksum)
	for _, digit := range digits {
		builder.WriteByte(base31Alphabet[digit])
	}
	raw := builder.String()
	return raw[:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:]
}

// Validate reports whether a UFI has 16 base-31 characters and a valid mod-31
// checksum. Dashes and spaces used for display are ignored.
func Validate(value string) bool {
	value = strings.NewReplacer("-", "", " ", "").Replace(value)
	if len(value) != 16 {
		return false
	}
	sum := 0
	for i := range value {
		digit := strings.IndexByte(base31Alphabet, value[i])
		if digit < 0 {
			return false
		}
		sum += (i + 1) * digit
	}
	return sum%31 == 0
}
