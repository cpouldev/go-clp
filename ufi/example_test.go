package ufi_test

import (
	"fmt"

	"github.com/cpouldev/go-clp/ufi"
)

func ExampleGenerate() {
	code, err := ufi.Generate("FR", "AB123456789", 123456)
	if err != nil {
		panic(err)
	}
	fmt.Println(code)
	fmt.Println(ufi.Validate(code))
	// Output:
	// 27W0-KC81-E00T-V314
	// true
}
