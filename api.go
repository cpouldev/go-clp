package clp

import core "github.com/cpouldev/go-clp/internal/core"

// Public data contracts are aliases of the internal deterministic engine. The
// aliases preserve the original root-package API while keeping its
// implementation and white-box tests out of the repository root.
type (
	FlagCode                = core.FlagCode
	FlagSeverity            = core.FlagSeverity
	Lang                    = core.Lang
	SdsStatus               = core.SdsStatus
	Flag                    = core.Flag
	Component               = core.Component
	ComponentHazard         = core.ComponentHazard
	ClassificationComponent = core.ClassificationComponent
	MixtureHazard           = core.MixtureHazard
	SdsLabel                = core.SdsLabel
	ADREntry                = core.ADREntry
	IATAEntry               = core.IATAEntry
	TransportResult         = core.TransportResult
	SdsSubsection           = core.SdsSubsection
	SdsSections             = core.SdsSections
	SdsPhysical             = core.SdsPhysical
	SdsLocaleSections       = core.SdsLocaleSections
	SdsClassification       = core.SdsClassification
	SdsMeta                 = core.SdsMeta
	SdsData                 = core.SdsData
	SdsPresentation         = core.SdsPresentation
	SdsPresentationLang     = core.SdsPresentationLang
	PictogramView           = core.PictogramView
	CodeText                = core.CodeText
	GlossaryEntry           = core.GlossaryEntry
	CompositionRow          = core.CompositionRow
	ClassComponent          = core.ClassComponent
	EngineInput             = core.EngineInput
	EngineResult            = core.EngineResult
	ProductClass            = core.ProductClass
	Harmonised              = core.Harmonised
	HarmonisedSCL           = core.HarmonisedSCL
	HarmonisedHazard        = core.HarmonisedHazard
	Recipe                  = core.Recipe
	RecipeLine              = core.RecipeLine
	SupplierSDS             = core.SupplierSDS
	ParsedExtraction        = core.ParsedExtraction
	ParsedSubstance         = core.ParsedSubstance
	ParsedHazard            = core.ParsedHazard
	HazardClass             = core.HazardClass
	MixtureClassification   = core.MixtureClassification
)

const (
	FlagDataMissing          = core.FlagDataMissing
	FlagUnitInconsistent     = core.FlagUnitInconsistent
	FlagSdsUnmapped          = core.FlagSdsUnmapped
	FlagNotClassified        = core.FlagNotClassified
	FlagArticleExcluded      = core.FlagArticleExcluded
	FlagRangeBased           = core.FlagRangeBased
	FlagNearThreshold        = core.FlagNearThreshold
	FlagNoHarmonisedLookup   = core.FlagNoHarmonisedLookup
	FlagEndpointNotEvaluated = core.FlagEndpointNotEvaluated
	FlagUndisclosedRemainder = core.FlagUndisclosedRemainder
	FlagProductClass         = core.FlagProductClass
	FlagAllergenCoverage     = core.FlagAllergenCoverage
	FlagInvalidHazardClass   = core.FlagInvalidHazardClass

	SeverityBlock = core.SeverityBlock
	SeverityWarn  = core.SeverityWarn
	SeverityInfo  = core.SeverityInfo

	LangEL = core.LangEL
	LangEN = core.LangEN

	SdsStatusReady  = core.SdsStatusReady
	SdsStatusFailed = core.SdsStatusFailed

	ClassUnknown  = core.ClassUnknown
	ClassLiquid   = core.ClassLiquid
	ClassSolidWax = core.ClassSolidWax
	ClassCosmetic = core.ClassCosmetic
	ClassSet      = core.ClassSet

	HazardClassAcuteToxicityOral        = core.HazardClassAcuteToxicityOral
	HazardClassAcuteToxicityDermal      = core.HazardClassAcuteToxicityDermal
	HazardClassAcuteToxicityInhalation  = core.HazardClassAcuteToxicityInhalation
	HazardClassSkinCorrosion            = core.HazardClassSkinCorrosion
	HazardClassSkinIrritation           = core.HazardClassSkinIrritation
	HazardClassSeriousEyeDamage         = core.HazardClassSeriousEyeDamage
	HazardClassEyeIrritation            = core.HazardClassEyeIrritation
	HazardClassSkinSensitisation        = core.HazardClassSkinSensitisation
	HazardClassRespiratorySensitisation = core.HazardClassRespiratorySensitisation
	HazardClassCarcinogenicity          = core.HazardClassCarcinogenicity
	HazardClassGermCellMutagenicity     = core.HazardClassGermCellMutagenicity
	HazardClassReproductiveToxicity     = core.HazardClassReproductiveToxicity
	HazardClassLactation                = core.HazardClassLactation
	HazardClassSTOTSingleExposure       = core.HazardClassSTOTSingleExposure
	HazardClassSTOTRepeatedExposure     = core.HazardClassSTOTRepeatedExposure
	HazardClassAspirationHazard         = core.HazardClassAspirationHazard
	HazardClassAquatic                  = core.HazardClassAquatic
	HazardClassAquaticAcute             = core.HazardClassAquaticAcute
	HazardClassAquaticChronic           = core.HazardClassAquaticChronic
	HazardClassFlammableLiquid          = core.HazardClassFlammableLiquid
	HazardClassPyrophoricSolid          = core.HazardClassPyrophoricSolid
	HazardClassExplosive                = core.HazardClassExplosive
	HazardClassOther                    = core.HazardClassOther
)

// RunEngine runs the deterministic classification pipeline.
func RunEngine(input EngineInput) EngineResult {
	return core.RunEngine(input)
}

// Classify derives the supported ADR/IATA transport classification.
func Classify(flashPoint *float64, aquaticClass, productType string, innerPackageLitres *float64) (TransportResult, []Flag) {
	return core.Classify(flashPoint, aquaticClass, productType, innerPackageLitres)
}

// ClassifySkinSens classifies skin sensitisation and EUH208 disclosure bands.
func ClassifySkinSens(components []ClassComponent) (string, []Flag) {
	return core.ClassifySkinSens(components)
}

// ClassifyAquatic applies the CLP aquatic summation cascade.
func ClassifyAquatic(components []ClassComponent) (string, []Flag) {
	return core.ClassifyAquatic(components)
}

// CheckFlammable reports whether the required flash point is available.
func CheckFlammable(flashPointKnown bool) []Flag {
	return core.CheckFlammable(flashPointKnown)
}

// ParseHazardClass validates a CLP hazard class and returns its canonical family.
func ParseHazardClass(class string) (HazardClass, error) {
	return core.ParseHazardClass(class)
}

// BuildPresentation resolves a classification into Greek and English label views.
func BuildPresentation(classification SdsClassification) SdsPresentation {
	return core.BuildPresentation(classification)
}

// ClassifyMixture classifies a concentration-resolved component list.
func ClassifyMixture(components []ClassComponent, flashPoint *float64, viscosityKnownLow bool) MixtureClassification {
	return core.ClassifyMixture(components, flashPoint, viscosityKnownLow)
}
