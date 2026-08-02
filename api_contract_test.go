package clp

import "testing"

func TestLibraryInputTypesUseDomainNames(t *testing.T) {
	recipe := Recipe{Name: "Test mixture", FormulationNumber: 7}
	line := RecipeLine{
		MaterialID:   "material-1",
		MaterialName: "Carrier",
		MaterialCode: "CAR-1",
		Quantity:     1.5,
		Unit:         "kg",
	}
	sds := SupplierSDS{MaterialID: line.MaterialID}

	if recipe.FormulationNumber != 7 || line.Quantity != 1.5 || sds.MaterialID != "material-1" {
		t.Fatalf("library input types lost their supplied values: recipe=%+v line=%+v sds=%+v", recipe, line, sds)
	}
}

func TestRunEngineCosmeticReturnsBlock(t *testing.T) {
	result := RunEngine(EngineInput{
		Recipe:       &Recipe{Name: "Body lotion", FormulationNumber: 1},
		ProductClass: ClassCosmetic,
	})

	for _, flag := range result.Flags {
		if flag.Code == FlagProductClass && flag.Severity == SeverityBlock {
			return
		}
	}
	t.Fatalf("RunEngine(ClassCosmetic) flags = %+v, want product-class BLOCK", result.Flags)
}
