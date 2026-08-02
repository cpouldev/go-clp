package core

// ProductClass selects the regulatory path for a finished product. Callers
// determine this value from their own catalogue or domain model and pass it to
// EngineInput; this package does not assume any category names or slugs.
type ProductClass int

const (
	// ClassUnknown runs the liquid path as a conservative compatibility default.
	ClassUnknown ProductClass = iota
	// ClassLiquid runs mixture classification, flash-point handling, and liquid
	// transport rules.
	ClassLiquid
	// ClassSolidWax skips liquid-only flash-point and transport rules.
	ClassSolidWax
	// ClassCosmetic stops classification because cosmetics are outside CLP.
	ClassCosmetic
	// ClassSet runs the liquid path best-effort; callers should normally classify
	// each finished product in a set separately.
	ClassSet
)

// String returns the stable machine label for c.
func (c ProductClass) String() string {
	switch c {
	case ClassLiquid:
		return "liquid"
	case ClassSolidWax:
		return "solid_wax"
	case ClassCosmetic:
		return "cosmetic"
	case ClassSet:
		return "set"
	default:
		return "unknown"
	}
}
