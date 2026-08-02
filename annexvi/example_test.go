package annexvi_test

import (
	"fmt"
	"time"

	"github.com/cpouldev/go-clp/annexvi"
)

func ExampleAsOf() {
	registry, err := annexvi.AsOf(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		panic(err)
	}
	for _, limit := range registry["7789-09-5"].SCLs {
		if limit.HCode == "H317" {
			fmt.Printf("%s: %.1f%%\n", limit.HCode, limit.Pct)
		}
	}
	// Output:
	// H317: 0.2%
}
