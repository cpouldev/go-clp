package core

import (
	"testing"
)

// method_table_test.go guards the method table as the single source of truth: it
// must cover every classifiable kind, assign each the correct method (the
// anti-"sum a per-component class" guarantee), and carry the CLP generic limits.

func TestMethodTable_CoversEveryClassifiableKind(t *testing.T) {
	for _, k := range []hazardKind{
		kindAcuteOral, kindAcuteDermal, kindAcuteInhal,
		kindSkinCorr, kindSkinIrrit, kindEyeDam, kindEyeIrrit,
		kindRespSens, kindCarc, kindMuta, kindRepr, kindLactation,
		kindStotSE, kindStotRE, kindAsp,
	} {
		if _, ok := endpointByKind[k]; !ok {
			t.Errorf("method table missing an entry for hazardKind %d", k)
		}
	}
}

func TestMethodTable_MethodAssignments(t *testing.T) {
	check := func(kinds []hazardKind, want classMethod) {
		t.Helper()
		for _, k := range kinds {
			m, ok := methodFor(k)
			if !ok {
				t.Errorf("no method declared for kind %d", k)
				continue
			}
			if m != want {
				t.Errorf("kind %d method = %s, want %s", k, m, want)
			}
		}
	}
	perComponent := []hazardKind{
		kindCarc, kindMuta, kindRepr, kindLactation, kindRespSens, kindSkinSens,
	}
	check(perComponent, methodPerComponentGCL)
	check(
		[]hazardKind{
			kindSkinCorr, kindSkinIrrit, kindEyeDam, kindEyeIrrit, kindStotSE, kindStotRE,
			kindAsp,
		},
		methodSummation,
	)
	check([]hazardKind{kindAcuteOral, kindAcuteDermal, kindAcuteInhal}, methodATE)
	check([]hazardKind{kindAquaticAcute, kindAquaticChronic}, methodAquaticCascade)

	// The root bug this refactor guards against: a per-component class must never
	// be summed.
	for _, k := range perComponent {
		if m, _ := methodFor(k); m == methodSummation {
			t.Errorf("per-component kind %d is wrongly marked as summation", k)
		}
	}
}

func TestMethodTable_GCLsMatchCLP(t *testing.T) {
	for _, c := range []struct {
		kind hazardKind
		cat  string
		want float64
	}{
		{kindCarc, "1", 0.1}, {kindCarc, "1A", 0.1}, {kindCarc, "2", 1.0},
		{kindMuta, "1B", 0.1}, {kindMuta, "2", 1.0},
		{kindRepr, "1", 0.3}, {kindRepr, "2", 3.0},
		{kindLactation, "*", 0.3},
		{kindSkinSens, "1", 1.0}, {kindSkinSens, "1A", 0.1}, {kindSkinSens, "1B", 1.0},
		{kindRespSens, "1", 1.0}, {kindRespSens, "1A", 0.1},
		{kindSkinCorr, "1", 5.0}, {kindSkinIrrit, "2", 10.0},
		{kindEyeDam, "1", 3.0}, {kindEyeIrrit, "2", 10.0},
		{kindStotSE, "1", 10.0}, {kindStotSE, "3", 20.0}, {kindStotRE, "1", 10.0},
		{kindAsp, "1", 10.0},
		{kindAquaticAcute, "*", 25.0}, {kindAquaticChronic, "*", 25.0},
	} {
		got, ok := gclFor(c.kind, c.cat)
		if !ok {
			t.Errorf("gclFor(%d,%q): no value", c.kind, c.cat)
			continue
		}
		if got != c.want {
			t.Errorf("gclFor(%d,%q) = %g, want %g", c.kind, c.cat, got, c.want)
		}
	}
	// ATE endpoints carry no % GCL.
	if _, ok := gclFor(kindAcuteOral, "2"); ok {
		t.Errorf("ATE endpoint should have no %% GCL")
	}
}

func TestEffectiveComponentLimit_SCLOverridesGCL(t *testing.T) {
	// A supplier SCL (0.2%) overrides the Repr. 2 GCL (3.0%) for this component.
	withSCL := ComponentHazard{Class: "Reproductive toxicity", Category: "2", SCLPct: 0.2}
	if limit, ok := effectiveComponentLimit(withSCL, kindRepr); !ok || limit != 0.2 {
		t.Errorf("effective limit = %g (ok=%v), want 0.2 (SCL overrides GCL)", limit, ok)
	}

	// Without an SCL the generic limit applies.
	noSCL := ComponentHazard{Class: "Reproductive toxicity", Category: "2"}
	if limit, _ := effectiveComponentLimit(noSCL, kindRepr); limit != 3.0 {
		t.Errorf("effective limit = %g, want GCL 3.0", limit)
	}
}
