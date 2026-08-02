package core

// engine.go contains the exported, pure deterministic classification pipeline.
// It depends only on recipe quantities and parsed supplier data and performs no
// I/O.

// EngineInput is the deterministic-classification input. It carries only
// structured, regulator-checkable data: the finished product identity, the
// recipe lines (whose quantities drive every concentration), and the parsed
// supplier extractions keyed by material ID. It holds no authored prose.
type EngineInput struct {
	// Recipe supplies the product name (transport product-type hint) and the
	// formulation number (UFI). Must be non-nil.
	Recipe *Recipe

	// CompanyCountry is the VAT-issuing country code used for UFI generation
	// (for example "GR", "FR", or "EU" for an ECHA company key).
	CompanyCountry string

	// CompanyVAT is the ISO-prefixed VAT number of the duty holder placing the
	// mixture on the market (e.g. "EL123456789"), used as the UFI issuer key.
	// Two companies with different VATs generate different UFIs for the same
	// formulation number, which is the whole point of the identifier — so this
	// is required whenever the mixture carries a UFI obligation. Empty raises a
	// data-missing FLAG instead of emitting a colliding UFI.
	CompanyVAT string

	// ProductClass routes the engine: ClassLiquid runs the full path; ClassSolidWax
	// skips the liquid flash-point classification and the SP375 liquid-transport
	// branch (a finished candle is not a flammable liquid); ClassSet/ClassUnknown
	// run the liquid path best-effort. The zero value (ClassUnknown) is a safe
	// liquid-path default. ClassCosmetic returns a blocking flag without running.
	ProductClass ProductClass

	// Lines are the recipe inputs. Component percentages are computed
	// as qty ÷ chemical-fraction-total, so the quantities here are decisive.
	Lines []RecipeLine

	// Extractions are the parsed supplier SDS results keyed by material ID
	// (the same key computeComposition/mixtureFlashPoint join on). A line with
	// no matching extraction classifies as an unhazardous filler at its full %.
	Extractions map[string]ParsedExtraction

	// InnerPackageLitres is the retail inner-packaging size in litres, fed to
	// the §14 SP375 decision. Nil means unknown: the transport core then treats
	// the product as SP375-exempt (consumer fragrance ships ≤ 5 L) and raises a
	// confirm-packaging review flag rather than over-classifying as UN 3082.
	InnerPackageLitres *float64

	// Harmonised is the CLP Annex VI harmonised classification registry keyed by
	// CAS (normCAS): substance-specific concentration limits and M-factors that
	// override the generic limits, per component. A non-nil map (even if
	// empty) means the registry was consulted, which arms the no_harmonised_lookup
	// validation flag for components with a relevant hazard and no entry. Nil means
	// no registry was available — no such flag is raised.
	Harmonised map[string]Harmonised

	// AllergenCAS is the set of CAS numbers (normCAS) on the EU Annex III
	// fragrance-allergen list supplied by the caller. The engine marks matching components and
	// the validation pass raises an allergen-coverage advisory for any present at a
	// disclosable level with no supplier sensitisation classification. Nil/empty =
	// not consulted (no such flag is raised). It never changes the classification.
	AllergenCAS map[string]bool

	// InciNames maps CAS (normCAS) → INCI cosmetic-nomenclature name supplied by
	// the caller. The engine
	// attaches the synonym to the matching component for the §3 render only; it
	// never changes the classification. Nil/empty = not consulted (no synonym
	// shown).
	InciNames map[string]string

	// SubstanceOverrides pins the exact finished-product concentration (% w/w) of a
	// substance, keyed by CAS (lower-case) or, when no CAS, the substance name
	// (lower-case). It REPLACES the worst-case upper bound the engine would
	// otherwise apportion from a supplier concentration RANGE, letting an operator
	// who has the real figure collapse a range-driven over-classification — e.g. a
	// Repr. 2 that only triggers at the range's upper bound (iso bornyl cyclohexanol
	// disclosed ≥3–<10% in the fragrance → 0.9–3.0% diluted; the real 1.x% drops
	// H361). It only narrows a range the supplier already declared, so it cannot
	// make the engine under-classify its own worst case. Nil/empty = none.
	SubstanceOverrides map[string]float64
}

// EngineResult is the fully deterministic classification produced by RunEngine.
// Every field is reproducible from EngineInput alone.
type EngineResult struct {
	// Composition is the % w/w audit table, one entry per chemical recipe line.
	Composition []Component

	// ClassComponents is the per-substance, concentration-resolved view the
	// classifier acted on (after apportioning supplier concentration ranges and
	// rolling up duplicate CAS numbers).
	ClassComponents []ClassComponent

	// FlashPointC is the lowest component flash point in °C, or nil when no
	// component reported one (which blocks the §14 flammable/not decision).
	FlashPointC *float64

	// AquaticClass is the summed aquatic classification code for the mixture
	// ("Aquatic Acute 1", "Aquatic Chronic 2", … or "" when not aquatic-toxic).
	AquaticClass string

	// Classification is the language-neutral hazard block: components, mixture
	// hazards, the GHS/CLP label, the UFI, and the §14 transport outcome.
	Classification SdsClassification

	// Presentation is the bilingual, render-ready view of §2/§3 derived from
	// Classification via the CLP phrase library.
	Presentation SdsPresentation

	// Flags is every FLAG raised by the deterministic stages, in pipeline order.
	Flags []Flag
}

// RunEngine runs the deterministic SDS classification pipeline end to end:
// composition from recipe quantities → finished-mixture label/hazards → guardrail
// self-check → UFI + §14 transport → bilingual presentation. It is pure: same
// input, same output, with no I/O.
func RunEngine(in EngineInput) EngineResult {
	if in.ProductClass == ClassCosmetic {
		return EngineResult{
			Flags: []Flag{
				{
					Code:     FlagProductClass,
					Severity: SeverityBlock,
					Message:  "Cosmetic products are outside this CLP classification engine; apply Regulation (EC) No 1223/2009 instead.",
				},
			},
		}
	}

	// Recipe carries the product name (transport product-type hint) and the
	// formulation number (UFI); both are read unconditionally below. Fail
	// closed rather than dereferencing nil.
	if in.Recipe == nil {
		return EngineResult{
			Flags: []Flag{
				{
					Section:  "1.1",
					Code:     FlagDataMissing,
					Severity: SeverityBlock,
					Message: "No recipe was supplied: EngineInput.Recipe must be non-nil to identify the product " +
						"and its formulation number.",
				},
			},
		}
	}

	var flags []Flag

	// Composition: pct = qty ÷ chemical-fraction total. Non-chemical articles
	// are excluded and mixed mass+volume without density raises unit_inconsistent.
	composition, compFlags := computeComposition(in.Lines)
	flags = append(flags, compFlags...)

	// Join composition to parsed hazard data by material ID; apportion
	// supplier concentration ranges; roll up duplicate substances by CAS.
	classComponents, rollupFlags := buildClassComponents(
		composition,
		in.Extractions,
		in.SubstanceOverrides,
	)
	flags = append(flags, rollupFlags...)

	// Override generic limits with harmonised SCLs / M-factors (CLP Annex VI) by
	// CAS, before any classification reads them. This is a no-op when no registry
	// was supplied; unmatched components are left for the validation pass to flag.
	for i := range classComponents {
		applyHarmonised(&classComponents[i], in.Harmonised)
		applyAllergenFlag(&classComponents[i], in.AllergenCAS)
		applyInciName(&classComponents[i], in.InciNames)
	}

	// Product-class routing: a solid wax (candle, wax melt) is NOT a flammable
	// liquid, so a measured constituent flash point must never push the finished
	// solid into Flam. Liq. nor drive its transport. The solid path classifies and
	// reports with no liquid flash point and routes §14 through the solid tree
	// (UN 3077, never Class 3); the liquid path keeps the measured value.
	isSolid := in.ProductClass == ClassSolidWax
	flashPt := mixtureFlashPoint(in.Lines, in.Extractions)
	if isSolid {
		flashPt = nil
	}

	// Finished-mixture classification — the Go engine is the source of truth for
	// the label and the "as a whole" hazard list. Viscosity is unknown here, so
	// aspiration is gated conservatively (the engine FLAGs it when relevant).
	mix := ClassifyMixture(classComponents, flashPt, false)
	flags = append(flags, mix.Flags...)
	label := mix.Label

	// Skin-sens core (EUH208 sub-threshold note) emits its own FLAGs; fold them
	// in. The flammable core (missing-flash-point block) is a LIQUID obligation
	// only — a solid has no flash point to be "missing", so it is skipped for the
	// solid-wax path.
	_, skinFlags := ClassifySkinSens(classComponents)
	flags = append(flags, skinFlags...)
	if !isSolid {
		flags = append(flags, CheckFlammable(flashPt != nil)...)
	}

	// UFI is generated only when a UFI / PCN obligation actually exists — i.e.
	// the mixture is classified for a health or physical hazard (CLP Annex VIII);
	// an environmental-only or not-classified mixture carries no UFI. The
	// formulation number is range-guarded inside generateUFI.
	aquaticClass, _ := ClassifyAquatic(classComponents)
	ufi, ufiFlags := generateUFI(in.Recipe, in.CompanyCountry, in.CompanyVAT, requiresUFI(label))
	flags = append(flags, ufiFlags...)

	// §14 transport, routed by physical state: a solid wax goes through the solid
	// tree (no flash-point branch, UN 3077 for an env hazard); everything else
	// runs the liquid tree.
	var transport TransportResult
	var transportFlags []Flag
	if isSolid {
		transport, transportFlags = classifySolidTransport(aquaticClass, in.InnerPackageLitres)
	} else {
		transport, transportFlags = Classify(
			flashPt,
			aquaticClass,
			productTypeHint(in.Recipe),
			in.InnerPackageLitres,
		)
	}
	flags = append(flags, transportFlags...)

	// Post-classification review pass: range-flip, near-threshold, and (when a
	// harmonised registry was consulted) missing-harmonised-lookup advisories
	// (sds_unmapped — components with no SDS on file — is raised upstream).
	flags = append(flags, validateClassification(classComponents, flashPt, in.Harmonised != nil)...)

	// not_classified (info) when neither a CLP label nor an aquatic class triggers.
	if !isClassified(label) && aquaticClass == "" {
		flags = append(
			flags, Flag{
				Code:     FlagNotClassified,
				Severity: SeverityInfo,
				Message:  "No hazard class is triggered: the mixture is not classified as hazardous. No UFI / Poison Centre Notification obligation applies.",
			},
		)
	}

	classification := SdsClassification{
		Components:     classComponentsToStored(classComponents),
		MixtureHazards: mix.Hazards,
		Label:          label,
		UFI:            ufi,
		Transport:      transport,
	}

	return EngineResult{
		Composition:     composition,
		ClassComponents: classComponents,
		FlashPointC:     flashPt,
		AquaticClass:    aquaticClass,
		Classification:  classification,
		Presentation:    BuildPresentation(classification),
		Flags:           flags,
	}
}
