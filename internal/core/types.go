package core

import "time"

// ── Flag taxonomy ────────────────────────────────────────────────────────────

// FlagCode is the machine-readable identifier for a FLAG. Every missing input,
// review note, or routing outcome maps to one of the codes defined below.
type FlagCode string

const (
	// FlagDataMissing fires when a required input for a deterministic computation
	// is absent (e.g. unknown flash point, SDS file too large to parse).
	FlagDataMissing FlagCode = "data_missing"

	// FlagUnitInconsistent fires when recipe lines mix mass and volume units
	// without a density value, making % w/w computation impossible.
	FlagUnitInconsistent FlagCode = "unit_inconsistent"

	// FlagSdsUnmapped is informational: one or more recipe materials had no
	// primary SDS file on record and were excluded from the mixture composition
	// (an SDS documents only substantiated supplier data). The % w/w is
	// re-baselined over the materials that have an SDS. Attach a primary SDS to a
	// material to include it.
	FlagSdsUnmapped FlagCode = "sds_unmapped"

	// FlagNotClassified is an informational flag set when no hazard class is
	// triggered. A "not classified" SDS is still valid; there is no PCN/UFI
	// obligation in that case.
	FlagNotClassified FlagCode = "not_classified"

	// FlagArticleExcluded is informational: one or more recipe lines were
	// non-chemical articles (counted by piece/length — packaging, wicks, ribbon)
	// and were excluded from the % w/w mixture composition, which is re-baselined
	// over the chemical fraction. No action required; the operator can confirm
	// nothing chemical was dropped.
	FlagArticleExcluded FlagCode = "article_excluded"

	// FlagRangeBased is a review flag from the validation pass: a classification
	// decision is triggered using the worst-case (upper) bound of a supplier
	// concentration range but would NOT trigger at the range's lower bound, so
	// the outcome hinges on the exact % w/w. Not an error — confirm the value.
	FlagRangeBased FlagCode = "range_based"

	// FlagNearThreshold is a review flag from the validation pass: a decision
	// sits within 10% (relative) of its CLP threshold, so a small concentration
	// revision could change the classification. Advisory only.
	FlagNearThreshold FlagCode = "near_threshold"

	// FlagNoHarmonisedLookup is a review flag from the validation pass: a
	// component carries a hazard whose classification can hinge on a
	// substance-specific concentration limit or M-factor (skin/respiratory
	// sensitisation, CMR, aquatic), yet no harmonised CLP Annex VI entry was found
	// for its CAS — so only the generic limit (or a supplier value) was applied.
	// Advisory: confirm there is no harmonised SCL/M-factor that would change the
	// outcome. Raised only when a harmonised registry was actually consulted.
	FlagNoHarmonisedLookup FlagCode = "no_harmonised_lookup"

	// FlagEndpointNotEvaluated is a review flag from the validation pass: a
	// component declares a hazard the engine could NOT turn into a classification
	// decision — a recognised endpoint with an unusable/absent category (so it was
	// silently dropped from its summation), or a hazard class the engine does not
	// classify at all. The danger is a silent non-classification; the operator must
	// supply the missing data or classify the endpoint manually.
	FlagEndpointNotEvaluated FlagCode = "endpoint_not_evaluated"

	// FlagUndisclosedRemainder is a review flag from the validation pass: the
	// disclosed component concentrations sum to well under 100%, so the engine is
	// treating a large unattributed remainder as non-hazardous. Confirm that the
	// undisclosed balance (carrier/solvent) really contains no hazardous substance.
	FlagUndisclosedRemainder FlagCode = "undisclosed_remainder"

	// FlagProductClass reports that the selected ProductClass requires caller
	// action, such as the ClassCosmetic hard stop.
	FlagProductClass FlagCode = "product_class"

	// FlagAllergenCoverage is a review flag from the validation pass: a component
	// on the EU Annex III fragrance-allergen list
	// is present at/above the generic EUH208 disclosure threshold but carries NO
	// supplier skin-sensitisation classification, so the engine's EUH208 author had
	// nothing to act on. Advisory (warn): confirm whether the substance needs
	// EUH208 "contains <name>" disclosure — the supplier classification may be
	// incomplete. Raised only when the allergen registry was consulted.
	FlagAllergenCoverage FlagCode = "allergen_coverage"
	// FlagInvalidHazardClass blocks release when a caller supplies a hazard
	// class outside the strict CLP vocabulary accepted by ParseHazardClass.
	FlagInvalidHazardClass FlagCode = "invalid_hazard_class"
)

// FlagSeverity indicates how a FLAG should be handled by the operator.
type FlagSeverity string

const (
	// SeverityBlock means the operator MUST resolve this FLAG before the SDS
	// can be considered trustworthy (e.g. unmapped SDS file, missing flash point
	// for a flammable assertion).
	SeverityBlock FlagSeverity = "block"

	// SeverityWarn means the operator SHOULD review this FLAG; the SDS may still
	// be issued under operator judgement.
	SeverityWarn FlagSeverity = "warn"

	// SeverityInfo is informational; no action required (e.g. not_classified).
	SeverityInfo FlagSeverity = "info"
)

// Lang identifies a supported rendered-text language.
type Lang string

const (
	LangEL Lang = "el"
	LangEN Lang = "en"
)

// SdsStatus is the lifecycle state of an SDS representation.
type SdsStatus string

const (
	SdsStatusReady  SdsStatus = "ready"
	SdsStatusFailed SdsStatus = "failed"
)

// ── FLAGS ────────────────────────────────────────────────────────────────────

// Flag is a single reviewer note attached to a generated SDS. It is surfaced
// both in the dash FLAG banner and, where Section is set, on the specific SDS
// section that triggered the issue.
type Flag struct {
	// Section is the optional REACH Annex II section number that the flag
	// relates to (e.g. "2.1", "9.1"). Omitted when the flag is document-wide.
	Section string `json:"section,omitempty"`

	// Code is the machine-readable flag identifier.
	Code FlagCode `json:"code"`

	// Severity tells callers whether the flag blocks, warns, or informs.
	Severity FlagSeverity `json:"severity"`

	// Message is a human-readable explanation shown to the operator.
	Message string `json:"message"`
}

// ── Composition audit ────────────────────────────────────────────────────────

// Component records one recipe line's deterministic composition data. Pct
// feeds the classifier; the remaining fields support an audit trail.
type Component struct {
	// MaterialID is the recipe line's material identifier. It is the join key
	// for EngineInput.Extractions: material NAMES are not unique — two lines can
	// carry two supplier batches of one trade name — so joining on the name
	// silently merges their SDS data.
	MaterialID string `json:"materialId"`

	// RawMaterialName is the human-readable material name — never a UUID.
	RawMaterialName string `json:"rawMaterialName"`

	// Code is the supplier/internal material code — never a UUID.
	Code string `json:"code"`

	// Qty is the supplied recipe-line quantity.
	Qty float64 `json:"qty"`

	// QtyUnit is the supplied unit string (e.g. "g", "ml", "kg").
	QtyUnit string `json:"qtyUnit"`

	// Pct is the computed % w/w (qty ÷ recipe total × 100). Zero until
	// composition has been calculated; nil-guard before classifier use.
	Pct float64 `json:"pct"`
}

// ── JSON shapes: classification block ────────────────────────────────────────

// ComponentHazard is one hazard entry for a component in the
// classification.components[].hazards list. Codes remain language-neutral;
// presentation helpers resolve phrase text separately.
type ComponentHazard struct {
	// Class is the CLP hazard class name (e.g. "Skin Sensitisation").
	Class string `json:"class"`

	// Category is the CLP category string (e.g. "1", "1A", "1B").
	Category string `json:"category"`

	// HCodes are the applicable H-statement codes (e.g. ["H317"]).
	HCodes []string `json:"hCodes"`

	// MFactorAcute is the M-factor for Aquatic Acute 1. Zero means not
	// applicable or the default of 1 applies.
	MFactorAcute int `json:"mFactorAcute,omitempty"`

	// MFactorChronic is the M-factor for Aquatic Chronic 1. Zero means not
	// applicable or the default of 1 applies.
	MFactorChronic int `json:"mFactorChronic,omitempty"`

	// SCLPct is the skin-sensitisation specific concentration limit (% w/w) when
	// the supplier lists one; zero means the generic limit applies. Carried here
	// so the per-substance hazard list fully feeds the dedicated classifier fields.
	SCLPct float64 `json:"sclPercent,omitempty"`
}

// ClassificationComponent is one entry in the classification.components list.
// It records the component's identity, the concentration the classifier used,
// and the list of per-substance hazard entries extracted from the supplier SDS.
type ClassificationComponent struct {
	// Name is the human-readable material name — never a UUID.
	Name string `json:"name"`

	// CasNumber is the CAS registry number (e.g. "140-11-4"). May be empty.
	CasNumber string `json:"casNumber,omitempty"`

	// EcNumber is the EC / EINECS number (e.g. "205-769-8"). May be empty.
	EcNumber string `json:"ecNumber,omitempty"`

	// InciName is the INCI (International Nomenclature of Cosmetic Ingredients)
	// name for the substance, when the ingredient registry carries one. It is a
	// §3 synonym shown for reader recognition (presentation only); it never feeds
	// the classification. Empty when no INCI name is on file.
	InciName string `json:"inciName,omitempty"`

	// ConcentrationPct is the computed % w/w used by the classifier (the
	// worst-case upper bound when apportioned from a supplier range).
	ConcentrationPct float64 `json:"concentrationPct"`

	// ConcentrationLowPct is the lower-bound % w/w; equals ConcentrationPct for
	// an exact concentration. Omitted when zero/equal.
	ConcentrationLowPct float64 `json:"concentrationLowPct,omitempty"`

	// FromRange is true when the concentration was apportioned from a supplier
	// range (the §3 band is then a worst-case disclosure).
	FromRange bool `json:"fromRange,omitempty"`

	// Hazards are the per-substance hazard entries the classifier acts on.
	Hazards []ComponentHazard `json:"hazards"`
}

// MixtureHazard is one entry in the classification.mixtureHazards list — the
// engine's hazard classification of the finished mixture itself.
type MixtureHazard struct {
	// Class is the CLP hazard class (e.g. "Flammable liquids").
	Class string `json:"class"`

	// Category is the assigned category (e.g. "3").
	Category string `json:"category"`

	// HCodes are the applicable H-codes for this mixture hazard.
	HCodes []string `json:"hCodes"`
}

// SdsLabel is the GHS/CLP label block within classification, computed by the
// deterministic engine (ClassifyMixture) and rendered into §2.
type SdsLabel struct {
	// Pictograms are the GHS pictogram codes that apply (e.g. ["GHS02", "GHS07"]).
	Pictograms []string `json:"pictograms"`

	// SignalWord is "Danger" or "Warning" (or "" for not classified).
	SignalWord string `json:"signalWord"`

	// HCodes are the H-statement codes for the finished mixture label.
	HCodes []string `json:"hCodes"`

	// EuhCodes are the EUH supplementary statement codes (e.g. ["EUH208"]).
	EuhCodes []string `json:"euhCodes"`

	// PCodes are the P-statement codes selected for the PHYSICAL LABEL — pruned
	// to the most relevant set (target ~6) via CLP Annex IV priority and
	// de-duplication of combined statements. The supplier/manufacturer chooses
	// the final label set; this is the engine's recommendation.
	PCodes []string `json:"pCodes"`

	// PCodesSds is the FULL set of triggered P-statement codes, shown in SDS
	// Section 2.2 (which may carry every triggered statement). PCodes is a
	// pruned subset of this list.
	PCodesSds []string `json:"pCodesSds"`

	// Euh208Substances names the skin-sensitising substances that are present
	// below their H317 classification limit but at/above their EUH208 band, so
	// the EUH208 statement can disclose them by name ("Contains <names>. …")
	// rather than the generic "contains a component" wording. Empty when EUH208
	// does not apply.
	Euh208Substances []string `json:"euh208Substances,omitempty"`
}

// ── Transport result ─────────────────────────────────────────────────────────

// ADREntry holds the road-transport (ADR/RID) fields for a regulated product.
type ADREntry struct {
	// UNNumber is the 4-digit UN identification number (e.g. "1266").
	UNNumber string `json:"unNumber"`

	// Class is the ADR danger class (e.g. "3").
	Class string `json:"class"`

	// PackingGroup is the packing group roman numeral (e.g. "III").
	PackingGroup string `json:"packingGroup"`

	// ProperShippingName is the official designation for the transport document.
	ProperShippingName string `json:"properShippingName"`

	// LimitedQuantityLitres is the limited-quantity (LQ) inner threshold in litres
	// (ADR: PG II → 1 L, PG III → 5 L for these Class 3/9 entries).
	LimitedQuantityLitres float64 `json:"limitedQuantityLitres"`

	// Regime is the dangerous-goods regime that applies given the inner-packaging
	// size: "full_dg", "limited_quantity" (LQ), or "excepted_quantity" (EQ). With
	// an unknown size it is "full_dg" (the conservative default) while the LQ/EQ
	// limit fields still report the thresholds the product is eligible for.
	Regime string `json:"regime"`

	// ExceptedQuantityCode is the ADR EQ code (e.g. "E1"/"E2") and ExceptedQuantity
	// Inner/OuterMl its receptacle limits in millilitres.
	ExceptedQuantityCode    string  `json:"exceptedQuantityCode,omitempty"`
	ExceptedQuantityInnerMl float64 `json:"exceptedQuantityInnerMl,omitempty"`
	ExceptedQuantityOuterMl float64 `json:"exceptedQuantityOuterMl,omitempty"`

	// SpecialProvision is the governing ADR special provision when one applies
	// (e.g. "SP375"). Empty when none does.
	SpecialProvision string `json:"specialProvision,omitempty"`
}

// IATAEntry holds the air-transport (IATA DGR) fields for a regulated product. It
// is computed independently of the ADR entry: the classification (UN/class/PG/PSN)
// is the same, but air is more restrictive, so the regime and the notes differ.
type IATAEntry struct {
	// UNNumber matches the ADR UN number.
	UNNumber string `json:"unNumber"`

	// Class is the IATA hazard division (mirrors ADR class for these products).
	Class string `json:"class"`

	// PackingGroup is the packing group (mirrors ADR for these products).
	PackingGroup string `json:"packingGroup"`

	// ProperShippingName is the IATA proper shipping name.
	ProperShippingName string `json:"properShippingName"`

	// LimitedQuantityLitres is the air LQ inner threshold in litres.
	LimitedQuantityLitres float64 `json:"limitedQuantityLitres"`

	// Regime is the air dangerous-goods regime that applies (see ADREntry.Regime).
	// It is derived from the ADR limited-quantity threshold, because this engine
	// does not carry the IATA DGR per-UN air limits — which are typically far
	// smaller. A product shippable under road LQ may still need full DG handling
	// by air, so Notes carries the caveat and the air leg must be confirmed
	// against the current DGR before shipping.
	Regime string `json:"regime"`

	// ExceptedQuantity* mirror the ADR fields — the EQ limits are harmonised with
	// the UN Model Regulations, so they apply to air as well.
	ExceptedQuantityCode    string  `json:"exceptedQuantityCode,omitempty"`
	ExceptedQuantityInnerMl float64 `json:"exceptedQuantityInnerMl,omitempty"`
	ExceptedQuantityOuterMl float64 `json:"exceptedQuantityOuterMl,omitempty"`

	// SpecialProvision is the governing IATA special provision (e.g. "A197", the
	// environmentally-hazardous-substance exemption equivalent to ADR SP375).
	SpecialProvision string `json:"specialProvision,omitempty"`

	// Notes carries an air-specific caveat for §14.6 — e.g. that flammable liquids
	// have stricter passenger/cargo-aircraft net-quantity limits under the current
	// IATA DGR edition. Empty when nothing air-specific applies.
	Notes string `json:"notes,omitempty"`
}

// TransportResult is the output of the fragrance decision tree. It is stored
// at classification.transport and rendered in SDS Section 14.
type TransportResult struct {
	// Regulated is false when no dangerous-goods classification applies
	// (e.g. low-hazard water-based products).
	Regulated bool `json:"regulated"`

	// ADR holds the road-transport fields. Nil when Regulated is false.
	ADR *ADREntry `json:"adr,omitempty"`

	// IATA holds the air-transport fields. Nil when Regulated is false.
	IATA *IATAEntry `json:"iata,omitempty"`

	// EnvMark is true when the environmentally-hazardous substance mark
	// (fish + tree symbol) must appear on the package. Required for UN 3082;
	// also required as an additional mark for Class 3 when aquatic
	// classification applies.
	EnvMark bool `json:"envMark"`

	// LqEligible is true when limited-quantity (LQ) exemption applies.
	LqEligible bool `json:"lqEligible"`

	// EqEligible is true when excepted-quantity (EQ) exemption applies.
	EqEligible bool `json:"eqEligible"`

	// Sp375Applied is true when the environmentally-hazardous substance
	// (UN 3082 / UN 3077) special provision SP375 exempts the product from
	// dangerous-goods regulation because the inner packaging is ≤ 5 L (liquids)
	// or ≤ 5 kg (solids). When true, Regulated is false and the ADR/IATA legs
	// are nil.
	Sp375Applied bool `json:"sp375Applied,omitempty"`

	// NotRegulatedReason is a short human-readable explanation (ENGLISH) shown in
	// Section 14 when Regulated is false but the product would otherwise be
	// classified (e.g. "Not regulated for transport (ADR SP375 / IATA
	// equivalent): environmentally hazardous substance in inner packaging
	// ≤ 5 L"). Empty for a genuinely non-hazardous product.
	NotRegulatedReason string `json:"notRegulatedReason,omitempty"`

	// NotRegulatedReasonEL is the Greek rendering of NotRegulatedReason, so the
	// §14 note is localised on the EL sheet rather than printing English inside an
	// otherwise-Greek document. Empty when there is no reason (mirrors
	// NotRegulatedReason). The print route picks this on the EL doc and falls back
	// to NotRegulatedReason.
	NotRegulatedReasonEL string `json:"notRegulatedReasonEl,omitempty"`
}

// ── Stable JSON shapes: top-level ─────────────────────────────────────────────

// SdsSubsection is one numbered REACH Annex II subsection within an advice
// section: its Annex II number (e.g. "10.1"), its localized heading, and the
// operator-editable body text. Num and Label come straight from the regulation's
// fixed structure and are rewritten deterministically on every run; only Text is
// meant to be hand-edited. An empty Text renders as the localised "not available"
// so the mandated subsection heading is always present even when there is no data
// for it (REACH Annex II requires every subsection to appear).
type SdsSubsection struct {
	Num   string `json:"num"`   // REACH Annex II subsection number, e.g. "10.1"
	Label string `json:"label"` // localized subsection heading
	Text  string `json:"text"`  // localized body; "" renders as "not available"
}

// SdsSections holds the operator-editable content for ONE language: the advice
// sections (REACH Annex II §4-8, §10-13, §15) plus the structured Section 9
// physical/chemical properties. Each advice section is the ordered list of its
// Annex II subsections (e.g. §10 → 10.1 Reactivity … 10.6 Hazardous decomposition
// products), seeded deterministically from the finished-mixture hazard codes
// (adviceForHazards); only §9 Physical is model-authored. Both remain
// operator-editable until the document is finalised. The data-driven sections
// (§1 identification, §2 hazards, §3 composition, §14 transport, §16 other) are
// NOT stored here — they are rendered from the deterministic Classification +
// Presentation blocks, so the model never re-narrates computed data. JSON keys
// keep the s<n> numbering the print route reads.
type SdsSections struct {
	S4       []SdsSubsection `json:"s4"`  // First aid measures
	S5       []SdsSubsection `json:"s5"`  // Firefighting measures
	S6       []SdsSubsection `json:"s6"`  // Accidental release measures
	S7       []SdsSubsection `json:"s7"`  // Handling and storage
	S8       []SdsSubsection `json:"s8"`  // Exposure controls / personal protection
	S10      []SdsSubsection `json:"s10"` // Stability and reactivity
	S11      []SdsSubsection `json:"s11"` // Toxicological information
	S12      []SdsSubsection `json:"s12"` // Ecological information
	S13      []SdsSubsection `json:"s13"` // Disposal considerations
	S15      []SdsSubsection `json:"s15"` // Regulatory information
	Physical SdsPhysical     `json:"physical"`
}

// SdsPhysical holds the Section 9 physical/chemical property values for one
// language. The fields are the full set of basic properties mandated by REACH
// Annex II §9.1 (as amended by Reg. (EU) 2020/878), in regulation order. Each is
// a short value string in the target language; an empty value renders as the
// localised "not available" placeholder so every mandated property row is present
// even when its value is unknown (the no-blank rule). The compose pass fills only
// the properties it can reliably determine and never guesses a number.
type SdsPhysical struct {
	PhysicalState           string `json:"physicalState"`           // e.g. "Υγρό" / "Liquid"
	Colour                  string `json:"colour"`                  // e.g. "Άχρωμο" / "Colourless"
	Odour                   string `json:"odour"`                   // characteristic odour description
	MeltingPointC           string `json:"meltingPointC"`           // melting / freezing point, °C
	BoilingPointC           string `json:"boilingPointC"`           // initial boiling point, °C
	Flammability            string `json:"flammability"`            // flammability (descriptive)
	ExplosionLimits         string `json:"explosionLimits"`         // lower / upper explosion limits
	FlashPointC             string `json:"flashPointC"`             // flash point, °C
	AutoIgnitionC           string `json:"autoIgnitionC"`           // auto-ignition temperature, °C
	DecompositionC          string `json:"decompositionC"`          // decomposition temperature, °C
	PH                      string `json:"ph"`                      // e.g. "δεν διατίθεται" / "7.0"
	Viscosity               string `json:"viscosity"`               // kinematic viscosity
	SolubilityWater         string `json:"solubilityWater"`         // solubility in water
	PartitionCoefficient    string `json:"partitionCoefficient"`    // n-octanol/water (log Kow)
	VapourPressure          string `json:"vapourPressure"`          // vapour pressure
	RelativeDensity         string `json:"relativeDensity"`         // density / relative density (water = 1)
	VapourDensity           string `json:"vapourDensity"`           // vapour density (air = 1)
	ParticleCharacteristics string `json:"particleCharacteristics"` // particle size (solids)
}

// SdsLocaleSections holds the bilingual editable section pair (deterministic
// advice + model-authored §9). Both languages are authored together in one
// compose run, so the §9 measurement values are identical across EL and EN.
type SdsLocaleSections struct {
	EL SdsSections `json:"el"`
	EN SdsSections `json:"en"`
}

// SdsClassification is the language-neutral hazard block. It is
// computed deterministically by the engine (RunEngine): components, mixture
// hazards, label, and transport, with the UFI set after UFI generation.
type SdsClassification struct {
	// Components is the per-substance classification breakdown. Each entry
	// mirrors one recipe line's extracted hazard data and computed concentration.
	Components []ClassificationComponent `json:"components"`

	// MixtureHazards is the engine's list of hazards for the finished mixture
	// (the "as a whole" classification derived from the component hazards and
	// concentrations by ClassifyMixture).
	MixtureHazards []MixtureHazard `json:"mixtureHazards"`

	// Label is the GHS/CLP label block — pictograms, signal word, H/EUH/P codes.
	Label SdsLabel `json:"label"`

	// UFI is the 16-character Unique Formula Identifier (`XXXX-XXXX-XXXX-XXXX`)
	// generated after the compose pass. Empty until computed.
	UFI string `json:"ufi,omitempty"`

	// Transport is the fragrance-scoped transport classification result.
	Transport TransportResult `json:"transport"`
}

// SdsMeta holds optional document-generation metadata and deterministic audit
// data for callers that persist complete SDS representations.
type SdsMeta struct {
	// Status is the lifecycle state of this SDS representation.
	Status SdsStatus `json:"status"`

	// Error holds the last generation error message when Status is "failed".
	Error string `json:"error,omitempty"`

	// Revision is the document revision number, starting at 1 on first issue and
	// incremented on every re-issue (REACH Annex II §16: a re-issued SDS must
	// carry an incremented revision and a revision date). A failed run preserves
	// the prior revision; a successful (re)generation or an out-of-scope notice
	// bumps it.
	Revision int `json:"revision"`

	// RevisedAt is the date of the current revision — set on every successful
	// (re)generation alongside Revision.
	RevisedAt *time.Time `json:"revisedAt,omitempty"`

	// LastRunAt is the time the most recent generation attempt started.
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`

	// GeneratedAt is the time the most recent successful generation completed.
	GeneratedAt *time.Time `json:"generatedAt,omitempty"`

	// Model is the LLM model identifier used for the compose pass
	// (e.g. "claude-opus-4-7").
	Model string `json:"model"`

	// EngineVersion is a monotonically-increasing string that identifies the
	// Go guardrail + prompt version. Bump when the regulatory logic changes
	// to allow re-generation detection.
	EngineVersion string `json:"engineVersion"`

	// Composition is the deterministic % w/w audit table, one entry per recipe
	// line, for verifying the concentrations used by the classifier.
	Composition []Component `json:"composition"`

	// Flags is the aggregated list of all FLAGs raised during generation.
	// Rendered in the dash FLAG banner and (where Section is set) on the
	// specific SDS section.
	Flags []Flag `json:"flags"`
}

// SdsData is a stable serialisation shape for one generated SDS.
//
// The document is split by authorship: Classification is the deterministic Go
// engine's source of truth (codes/categories); Presentation is the bilingual,
// render-ready view of the data-driven sections (§2 label, §3 composition)
// derived from Classification via the CLP phrase library at generate time, so
// the print route renders stored text without a phrase library of its own;
// Sections holds the deterministic, operator-editable advice + the §9 values.
type SdsData struct {
	Meta           SdsMeta           `json:"meta"`
	Classification SdsClassification `json:"classification"`
	Presentation   SdsPresentation   `json:"presentation"`
	Sections       SdsLocaleSections `json:"sections"`
}

// ── Stable JSON shapes: presentation (derived, bilingual, render-ready) ───────

// SdsPresentation is the bilingual, render-ready view of the data-driven
// sections. It is derived deterministically from Classification and the CLP
// phrase library at generate time (both languages, regardless of which language
// the advice was authored in this turn), so the print route is a dumb renderer.
type SdsPresentation struct {
	EL SdsPresentationLang `json:"el"`
	EN SdsPresentationLang `json:"en"`
}

// SdsPresentationLang is one language's resolved label + composition view.
type SdsPresentationLang struct {
	// SignalWord is the localised signal word ("Κίνδυνος"/"Danger", ""/none).
	SignalWord string `json:"signalWord"`

	// Pictograms are the GHS pictogram codes with localised alt text, in code order.
	Pictograms []PictogramView `json:"pictograms"`

	// HStatements / EuhStatements / PStatements pair each label code with its
	// resolved statement text in this language.
	HStatements   []CodeText `json:"hStatements"`
	EuhStatements []CodeText `json:"euhStatements"`

	// PStatements is the pruned PHYSICAL-LABEL P-statement set (target ~6),
	// resolved to text. PStatementsSds is the FULL SDS Section 2.2 set. The
	// print route renders PStatements on the label panel and PStatementsSds in
	// Section 2.2.
	PStatements    []CodeText `json:"pStatements"`
	PStatementsSds []CodeText `json:"pStatementsSds"`

	// Components are the Section 3 composition rows (hazardous substances only).
	Components []CompositionRow `json:"components"`

	// Glossary is every label code (H, then EUH, then P; de-duplicated) paired
	// with its official statement text and, for H-codes that carry one, the GHS
	// pictogram code. The print route renders codes compactly on the §2 label and
	// expands them here at the end of the document, so a code shown inline always
	// has an authoritative entry. Empty when the mixture is not classified.
	Glossary []GlossaryEntry `json:"glossary"`
}

// PictogramView is one GHS pictogram code with its localised alt text.
type PictogramView struct {
	Code string `json:"code"` // e.g. "GHS07"
	Alt  string `json:"alt"`  // localised name, e.g. "Θαυμαστικό" / "Exclamation mark"
}

// CodeText pairs a CLP statement code with its resolved text in one language.
type CodeText struct {
	Code string `json:"code"` // e.g. "H317"
	Text string `json:"text"` // resolved statement text
}

// GlossaryEntry is one end-of-document glossary row: a CLP code, its official
// statement text in the document language, and (for H-codes that carry one) the
// GHS pictogram code so the print route can show the icon beside the entry.
// Pictogram is empty for EUH / P codes and for the no-pictogram H-codes.
type GlossaryEntry struct {
	Code      string `json:"code"`                // e.g. "H317", "EUH208", "P305+P351+P338"
	Text      string `json:"text"`                // official statement text in the doc language
	Pictogram string `json:"pictogram,omitempty"` // e.g. "GHS07"; empty when none
}

// CompositionRow is one Section 3 hazardous-substance row. The concentration is
// shown as a standard CLP band (never the exact trade-secret % w/w); the
// classification cell lists the substance's hazard classes and H-codes.
type CompositionRow struct {
	Name               string `json:"name"`
	CasNumber          string `json:"casNumber,omitempty"`
	EcNumber           string `json:"ecNumber,omitempty"`
	InciName           string `json:"inciName,omitempty"` // §3 synonym (cosmetic nomenclature); presentation only
	ConcentrationRange string `json:"concentrationRange"` // e.g. "≥ 1 - < 5 %"
	Classification     string `json:"classification"`     // e.g. "Skin Sens. 1B (H317)"
}

// ── Pure-core input DTOs ──────────────────────────────────────────────────────

// ClassComponent is the input DTO for the pure-core classifier functions
// (ClassifySkinSens, ClassifyAquatic, ClassifyMixture). It carries only the
// fields the classifier needs, without persistence-layer identifiers.
type ClassComponent struct {
	// Name is the human-readable material name used in FLAG messages.
	Name string

	// ConcentrationPct is the computed % w/w (0–100 scale). When the
	// concentration was apportioned from a supplier range this carries the
	// worst-case (UPPER) bound, which is what every hazard-trigger decision
	// uses (CLP safe direction).
	ConcentrationPct float64

	// ConcentrationLowPct is the lower-bound % w/w. It equals ConcentrationPct
	// for an exact concentration; for a range it is the apportioned LOWER
	// bound. The validation pass re-runs the classification at the lower bounds
	// to detect decisions that would flip between the range ends.
	ConcentrationLowPct float64

	// FromRange is true when this component's concentration was apportioned
	// from a supplier concentration range (so the trigger used the upper bound
	// and the result may be "range-based, confirm exact %").
	FromRange bool

	// SkinSensCategory is the CLP skin sensitisation category string for this
	// substance. One of: "1", "1A", "1B", or "" (not a skin sensitiser).
	SkinSensCategory string

	// SkinSensSCL is the substance-specific concentration limit (SCL) in %w/w
	// for skin sensitisation, if one is listed in the harmonised classification.
	// Zero means no SCL; the generic concentration limit (GCL) applies.
	SkinSensSCLPct float64

	// AquaticAcuteCategory is the CLP acute aquatic category string: "1", or "".
	AquaticAcuteCategory string

	// AquaticChronicCategory is the CLP chronic aquatic category string:
	// "1", "2", "3", "4", or "".
	AquaticChronicCategory string

	// MAcute is the M-factor for Aquatic Acute 1. 0 means use default (1).
	MAcute int

	// MChronic is the M-factor for Aquatic Chronic 1. 0 means use default (1).
	MChronic int

	// CasNumber is the component's primary CAS number, when extracted. Carried
	// for the composition table; not used by the legacy skin-sens/aquatic cores.
	CasNumber string

	// HarmonisedFound records that a CLP Annex VI harmonised entry was matched for
	// this component's CAS and applied (its SCL/M-factor overriding the generic
	// limit). False means only the generic limit (or a supplier value) was used;
	// the validation pass flags that for hazards where it could matter.
	HarmonisedFound bool

	// IsFragranceAllergen records that this component's CAS is on the EU Annex III
	// fragrance-allergen list. The validation
	// pass uses it for allergen-disclosure coverage (FlagAllergenCoverage); it does
	// not change the classification or label.
	IsFragranceAllergen bool

	// EcNumber is the component's primary EC / EINECS number, when extracted.
	// Carried for the Section 3 composition table; not used by the cores.
	EcNumber string

	// InciName is the INCI cosmetic-nomenclature synonym for the component,
	// supplied by the caller and keyed by CAS. It is carried for the Section 3
	// synonym render only; no
	// classifier reads it. Empty when none is on file.
	InciName string

	// Euh holds the supplier-declared EUH codes for the component's substances
	// (e.g. EUH204 isocyanates, EUH205 epoxy constituents, EUH401). The label
	// pipeline propagates the constituent-disclosure ones onto the mixture label;
	// EUH208 is excluded there (the engine authors it itself, with names).
	Euh []string

	// Hazards is the full per-component CLP hazard list (class + category +
	// H-codes + M-factors) extracted from the supplier SDS. ClassifyMixture reads
	// this to evaluate every hazard class (acute tox, irritation, CMR, STOT,
	// aspiration, respiratory sens, …) beyond the three legacy guardrail classes,
	// which still read the dedicated category fields above.
	Hazards []ComponentHazard
}
