package allergens_test

import (
	"fmt"

	"github.com/cpouldev/go-clp/allergens"
)

func ExampleLoad() {
	registry, err := allergens.Load()
	if err != nil {
		panic(err)
	}

	fmt.Println(registry.CAS["100-51-6"])
	fmt.Println(registry.INCIByCAS["100-51-6"])
	// Output:
	// true
	// Benzyl Alcohol
}

func ExampleRequiresDisclosure() {
	required, err := allergens.RequiresDisclosure(allergens.LeaveOn, 0.0011)
	if err != nil {
		panic(err)
	}
	fmt.Println(required)
	// Output: true
}
