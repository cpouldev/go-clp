package core

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
)

// reference_sample_test.go reproduces the first real product SDS this engine
// generated — a finished home-fragrance: a fragrance concentrate dosed at ~30%
// into a 70% Solketal (2,2-dimethyl-1,3-dioxolan-4-ylmethanol) carrier — directly
// from the 23 resolved components that were stored in its classification, and
// asserts the CORRECTED post-compliance-fix behaviour. It is both the regression
// guard for the reference product and the CHECK artefact of the compliance PDCA
// cycle (run `go test -run TestReferenceSample -v` to print
// the pre-fix → post-fix diff).
//
// Pre-fix (stored) output, for reference:
//
//	pictograms  GHS07, GHS08, GHS09          (GHS09 wrong — mixture is only Chronic 2)
//	euhCodes    EUH208                       (unnamed)
//	pCodes      17 codes                     (full set, not pruned for the label)
//	transport   UN 3082 Class 9 PG III       (regulated, env mark — ignored SP375)
//
// Post-fix expectations are asserted below.

func refHz(class, category string, hCodes ...string) ComponentHazard {
	return ComponentHazard{Class: class, Category: category, HCodes: hCodes}
}

func refComp(name string, pct float64, hazards ...ComponentHazard) ClassComponent {
	c := ClassComponent{Name: name, ConcentrationPct: pct, ConcentrationLowPct: pct, Hazards: hazards}
	applyDedicatedFields(&c)
	return c
}

func refSubstrIn(list []string, needle string) bool {
	for _, s := range list {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func refHasFlag(flags []Flag, code FlagCode) bool {
	for _, f := range flags {
		if f.Code == code {
			return true
		}
	}
	return false
}

// referenceSampleComponents returns the 23 resolved components exactly as stored
// (chemical names normalised to English; the canonical H-code per hazard class).
func referenceSampleComponents() []ClassComponent {
	const (
		skinSens = "Skin sensitisation"
		skinIrr  = "Skin irritation"
		eyeIrr   = "Eye irritation"
		repr     = "Reproductive toxicity"
		acuteTox = "Acute toxicity"
		asp      = "Aspiration hazard"
		aqAcute  = "Hazardous to the aquatic environment - acute"
		aqChron  = "Hazardous to the aquatic environment - chronic"
	)
	return []ClassComponent{
		refComp(
			"2,2-Dimethyl-1,3-dioxolan-4-ylmethanol (Solketal)", 70,
			refHz(eyeIrr, "2", "H319"),
		),
		refComp(
			"ISO E SUPER", 3,
			refHz(aqChron, "1", "H410"), refHz(skinIrr, "2", "H315"), refHz(skinSens, "1", "H317"),
		),
		refComp(
			"Linalyl acetate", 3,
			refHz(skinIrr, "2", "H315"), refHz(eyeIrr, "2", "H319"), refHz(skinSens, "1", "H317"),
		),
		refComp(
			"Galaxolide (HHCB)", 3,
			refHz(aqAcute, "1", "H400"), refHz(aqChron, "1", "H410"),
		),
		refComp(
			"Fixolid", 3,
			refHz(aqAcute, "1", "H400"), refHz(aqChron, "1", "H410"), refHz(acuteTox, "4", "H302"),
		),
		refComp(
			"Coumarin", 3,
			refHz(acuteTox, "4", "H302"), refHz(skinSens, "1B", "H317"), refHz(aqChron, "3", "H412"),
		),
		refComp(
			"Linalool", 3,
			refHz(skinIrr, "2", "H315"), refHz(eyeIrr, "2", "H319"), refHz(skinSens, "1B", "H317"),
		),
		// ≥3 - <10% in the BOOKSTORE concentrate (Vioryl) × 30% blend = < 3.0% in the
		// finished diffuser — strictly below the 3% Repr. 2 GCL, so H361 does NOT
		// classify. Modelled as the worst case just below 3.0%, exactly as
		// buildClassComponents stores a strict "<10%" upper bound.
		refComp(
			"Iso bornyl cyclohexanol",
			3-strictBoundMargin,
			refHz(repr, "2", "H361"),
			refHz(aqAcute, "1", "H400"),
			refHz(aqChron, "2", "H411"),
			refHz(eyeIrr, "2", "H319"),
		),
		refComp(
			"Acetyl cedrene (Vertofix)", 3,
			refHz(aqAcute, "1", "H400"), refHz(aqChron, "1", "H410"), refHz(skinSens, "1", "H317"),
		),
		refComp(
			"Cedryl acetate", 0.75,
			refHz(aqAcute, "1", "H400"), refHz(aqChron, "1", "H410"), refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Methyl ionone",
			0.75,
			refHz(aqChron, "2", "H411"),
			refHz(skinIrr, "2", "H315"),
			refHz(eyeIrr, "2", "H319"),
			refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Cedrol methyl ether", 0.75,
			refHz(aqAcute, "1", "H400"), refHz(aqChron, "1", "H410"), refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Juniperus Mexicana oil (Cedarwood Texas)",
			0.75,
			refHz(asp, "1", "H304"),
			refHz(aqAcute, "1", "H400"),
			refHz(aqChron, "1", "H410"),
			refHz(skinIrr, "2", "H315"),
			refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Eugenol", 0.75,
			refHz(eyeIrr, "2", "H319"), refHz(skinSens, "1", "H317"),
		),
		refComp(
			"p-tert butyl cyclohexyl acetate", 0.75,
			refHz(skinSens, "1", "H317"),
		),
		refComp(
			"Hydrogenated methyl rosinate", 0.75,
			refHz(aqChron, "3", "H412"),
		),
		refComp(
			"Piperonal", 0.75,
			refHz(repr, "2", "H361"), refHz(skinSens, "1", "H317"),
		),
		refComp(
			"Ethyl maltol", 0.75,
			refHz(acuteTox, "4", "H302"),
		),
		refComp(
			"Patchouli oil", 0.75,
			refHz(asp, "1", "H304"), refHz(aqChron, "2", "H411"), refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Isolongifolene ketone (Piconia)", 0.3,
			refHz(aqChron, "2", "H411"), refHz(skinIrr, "2", "H315"), refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Mousse coeur", 0.3,
			refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Gurjun oil",
			0.3,
			refHz(asp, "1", "H304"),
			refHz(aqAcute, "1", "H400"),
			refHz(aqChron, "1", "H410"),
			refHz(skinSens, "1B", "H317"),
		),
		refComp(
			"Olibanum resinoid",
			0.3,
			refHz(asp, "1", "H304"),
			refHz(aqChron, "2", "H411"),
			refHz(skinIrr, "2", "H315"),
			refHz(skinSens, "1B", "H317"),
		),
	}
}

func TestReferenceSample_CompliantClassification(t *testing.T) {
	comps := referenceSampleComponents()

	// The Solketal carrier flashes ~80 °C (well above the 60 °C Class-3 ceiling),
	// so the mixture is non-flammable — reproducing the original non-flammable,
	// environmentally-hazardous transport path.
	flashPt := 80.0

	mix := ClassifyMixture(comps, &flashPt, false)
	label := mix.Label
	aquaticClass, _ := ClassifyAquatic(comps)

	// Consumer retail pack: inner-packaging size unknown → SP375 assumes ≤5 L.
	transport, transportFlags := Classify(&flashPt, aquaticClass, "finished", nil)
	reviewFlags := validateClassification(comps, &flashPt, false)

	// ── Aquatic: Chronic 2 (10×ΣC·M_chron1 + ΣC_chron2 ≥ 25) carries no GHS09 ──
	if aquaticClass != "Aquatic Chronic 2" {
		t.Errorf("aquaticClass = %q, want Aquatic Chronic 2", aquaticClass)
	}
	if slices.Contains(label.Pictograms, "GHS09") {
		t.Errorf("Aquatic Chronic 2 must NOT carry GHS09; pictograms = %v", label.Pictograms)
	}

	// ── Pictograms: GHS07 only. Repr. 2 does NOT apply (iso bornyl cyclohexanol is
	// < 3% in the diluted product), so no GHS08; Aquatic Chronic 2 carries no GHS09. ─
	if !slices.Contains(label.Pictograms, "GHS07") {
		t.Errorf("pictograms %v missing GHS07", label.Pictograms)
	}
	if slices.Contains(label.Pictograms, "GHS08") {
		t.Errorf("Repr. 2 must not apply (iso bornyl < 3%% diluted) → no GHS08; got %v", label.Pictograms)
	}

	// ── Signal word: Warning (no Danger-level hazard present) ──────────────────
	if label.SignalWord != "Warning" {
		t.Errorf("signalWord = %q, want Warning", label.SignalWord)
	}

	// ── H-codes: H317 + H315 + H319 + H411 (Repr. 2 / H361 dropped — see above) ──
	for _, want := range []string{"H317", "H315", "H319", "H411"} {
		if !slices.Contains(label.HCodes, want) {
			t.Errorf("hCodes %v missing %s", label.HCodes, want)
		}
	}
	if slices.Contains(label.HCodes, "H361") {
		t.Errorf(
			"Repr. 2 (H361) must not classify: iso bornyl cyclohexanol is < 3%% in the diluted product; got %v",
			label.HCodes,
		)
	}

	// ── EUH208 NAMES the sub-threshold sensitisers; suppresses the ≥1% ones ────
	if !slices.Contains(label.EuhCodes, "EUH208") {
		t.Errorf("euhCodes %v missing EUH208", label.EuhCodes)
	}
	for _, name := range []string{"Eugenol", "Cedryl acetate", "Mousse coeur", "Patchouli oil"} {
		if !refSubstrIn(label.Euh208Substances, name) {
			t.Errorf("EUH208 should NAME sub-threshold sensitiser %q; got %v", name, label.Euh208Substances)
		}
	}
	for _, name := range []string{"Coumarin", "Linalool", "ISO E SUPER", "Acetyl cedrene", "Linalyl acetate"} {
		if refSubstrIn(label.Euh208Substances, name) {
			t.Errorf("EUH208 must SUPPRESS %q (≥1%% → already H317); got %v", name, label.Euh208Substances)
		}
	}

	// ── P-statements: pruned label set ≤ cap; SDS §2.2 set is the full superset ─
	if len(label.PCodes) > labelPStatementCap {
		t.Errorf("label P-statements = %d %v, want ≤ %d", len(label.PCodes), label.PCodes, labelPStatementCap)
	}
	if len(label.PCodesSds) <= len(label.PCodes) {
		t.Errorf(
			"SDS §2.2 P-statements (%d) must exceed the pruned label set (%d)",
			len(label.PCodesSds),
			len(label.PCodes),
		)
	}

	// ── §14 transport: SP375 exemption → not regulated + confirm-packaging flag ─
	if transport.Regulated {
		t.Errorf("consumer pack (≤5 L) → SP375 exempt, want NOT regulated; got %+v", transport)
	}
	if !transport.Sp375Applied {
		t.Errorf("SP375 exemption must be recorded on the transport result")
	}
	if transport.EnvMark {
		t.Errorf("SP375 exemption removes the transport env-hazard mark")
	}
	if !refHasFlag(transportFlags, FlagDataMissing) {
		t.Errorf("unknown package under SP375 must raise a confirm-packaging flag; got %v", transportFlags)
	}

	// ── Validation pass: Repr. 2 sits AT the 3% GCL → near-threshold review flag ─
	if !refHasFlag(reviewFlags, FlagNearThreshold) {
		t.Errorf("expected a near_threshold review flag (Repr. 2 at the 3%% limit); got %v", reviewFlags)
	}

	t.Log(referenceSampleDiff(label, aquaticClass, transport, reviewFlags))
}

func referenceSampleDiff(
	label SdsLabel,
	aquaticClass string,
	transport TransportResult,
	reviewFlags []Flag,
) string {
	sorted := func(s []string) string {
		c := append([]string(nil), s...)
		sort.Strings(c)
		return strings.Join(c, ", ")
	}
	transportLine := func(t TransportResult) string {
		if t.Regulated && t.ADR != nil {
			return fmt.Sprintf(
				"UN %s Class %s PG %s (regulated, env mark=%v)",
				t.ADR.UNNumber,
				t.ADR.Class,
				t.ADR.PackingGroup,
				t.EnvMark,
			)
		}
		return fmt.Sprintf(
			"NOT regulated (SP375 applied=%v, env mark=%v) — %s",
			t.Sp375Applied,
			t.EnvMark,
			t.NotRegulatedReason,
		)
	}

	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	p("")
	p("REFERENCE SAMPLE — pre-fix (stored)  →  post-fix (recomputed)")
	p("──────────────────────────────────────────────────────────────")
	p("Pictograms : [GHS07 GHS08 GHS09]     →  [%s]", sorted(label.Pictograms))
	p("Signal word: Warning                 →  %s", label.SignalWord)
	p("H-codes    : H315 H317 H319 H361 H411 →  [%s]", sorted(label.HCodes))
	p(
		"EUH        : EUH208 (unnamed)        →  %s, names: %s",
		strings.Join(label.EuhCodes, ","),
		strings.Join(label.Euh208Substances, "; "),
	)
	p("Label P    : 17 codes (unpruned)     →  %d codes: %s", len(label.PCodes), strings.Join(label.PCodes, ", "))
	p("SDS §2.2 P : (not separated)         →  %d codes (full set)", len(label.PCodesSds))
	p("Aquatic    : Aquatic Chronic 2       →  %s", aquaticClass)
	p("Transport  : UN 3082 Class 9 PG III (regulated, env mark)")
	p("           →  %s", transportLine(transport))
	p("Review pass: (none)                  →  %d flag(s):", len(reviewFlags))
	for _, f := range reviewFlags {
		p("             • [%s/%s] %s", f.Code, f.Severity, f.Message)
	}
	return b.String()
}
