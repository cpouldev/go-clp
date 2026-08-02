package clp_test

import (
	"fmt"

	clp "github.com/cpouldev/go-clp"
)

func ExampleClassifyMixture() {
	result := clp.ClassifyMixture(
		[]clp.ClassComponent{
			{
				Name:                "sensitising fragrance",
				ConcentrationPct:    1.2,
				ConcentrationLowPct: 1.2,
				SkinSensCategory:    "1B",
				Hazards: []clp.ComponentHazard{
					{Class: "Skin sensitisation", Category: "1B", HCodes: []string{"H317"}},
				},
			},
		}, nil, false,
	)

	fmt.Println(result.Label.SignalWord)
	fmt.Println(result.Label.Pictograms)
	fmt.Println(result.Label.HCodes)
	// Output:
	// Warning
	// [GHS07]
	// [H317]
}

func ExampleBuildPresentation() {
	presentation := clp.BuildPresentation(
		clp.SdsClassification{
			Label: clp.SdsLabel{SignalWord: "Warning", HCodes: []string{"H317"}},
		},
	)

	fmt.Println(presentation.EN.SignalWord)
	fmt.Println(presentation.EN.HStatements[0].Text)
	fmt.Println(presentation.EL.SignalWord)
	fmt.Println(presentation.EL.HStatements[0].Text)
	// Output:
	// Warning
	// May cause an allergic skin reaction.
	// Προσοχή
	// Μπορεί να προκαλέσει αλλεργική δερματική αντίδραση.
}
