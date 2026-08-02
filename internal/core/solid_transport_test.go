package core

import (
	"testing"
)

func TestClassifySolidTransport(t *testing.T) {
	t.Run(
		"no aquatic hazard is not regulated", func(t *testing.T) {
			res, flags := classifySolidTransport("", ptrF(0.2))
			if res.Regulated {
				t.Errorf("expected not regulated, got %+v", res)
			}
			if res.ADR != nil || res.IATA != nil {
				t.Errorf("expected no ADR/IATA legs, got ADR=%+v IATA=%+v", res.ADR, res.IATA)
			}
			if len(flags) != 0 {
				t.Errorf("expected no flags, got %+v", flags)
			}
		},
	)

	t.Run(
		"aquatic chronic 3 does not trigger transport env hazard", func(t *testing.T) {
			// Only Acute 1 / Chronic 1 / Chronic 2 are transport env hazards.
			res, _ := classifySolidTransport("Aquatic Chronic 3", ptrF(0.2))
			if res.Regulated {
				t.Errorf("expected not regulated for Chronic 3, got %+v", res)
			}
		},
	)

	t.Run(
		"env hazard ≤5kg is SP375-exempt with known size", func(t *testing.T) {
			res, flags := classifySolidTransport("Aquatic Chronic 1", ptrF(0.2))
			if res.Regulated {
				t.Errorf("expected not regulated (SP375), got %+v", res)
			}
			if !res.Sp375Applied {
				t.Error("expected Sp375Applied = true")
			}
			if res.EnvMark {
				t.Error("SP375 exemption must drop the env mark")
			}
			if len(flags) != 0 {
				t.Errorf("known size ≤5kg should raise no flag, got %+v", flags)
			}
		},
	)

	t.Run(
		"env hazard unknown size is SP375-exempt with a confirm flag", func(t *testing.T) {
			res, flags := classifySolidTransport("Aquatic Acute 1", nil)
			if res.Regulated {
				t.Errorf("expected not regulated (SP375), got %+v", res)
			}
			if !res.Sp375Applied {
				t.Error("expected Sp375Applied = true")
			}
			if len(flags) != 1 || flags[0].Code != FlagDataMissing || flags[0].Section != "14" {
				t.Fatalf("expected one §14 data_missing flag, got %+v", flags)
			}
			if flags[0].Severity != SeverityWarn {
				t.Errorf("expected warn severity, got %q", flags[0].Severity)
			}
		},
	)

	t.Run(
		"env hazard >5kg ships as UN 3077 Class 9", func(t *testing.T) {
			res, flags := classifySolidTransport("Aquatic Chronic 1", ptrF(10.0))
			if !res.Regulated {
				t.Fatalf("expected regulated, got %+v", res)
			}
			if res.ADR == nil || res.ADR.UNNumber != "3077" || res.ADR.Class != "9" || res.ADR.PackingGroup != "III" {
				t.Errorf("ADR = %+v, want UN 3077 Class 9 PG III", res.ADR)
			}
			if res.ADR.ProperShippingName != psnUN3077 {
				t.Errorf("ADR PSN = %q, want %q", res.ADR.ProperShippingName, psnUN3077)
			}
			if res.IATA == nil || res.IATA.UNNumber != "3077" {
				t.Errorf("IATA = %+v, want UN 3077", res.IATA)
			}
			if !res.EnvMark {
				t.Error("UN 3077 requires the environmentally-hazardous mark")
			}
			if res.Sp375Applied {
				t.Error("Sp375Applied must be false above 5 kg")
			}
			if len(flags) != 0 {
				t.Errorf("expected no flags for a clear UN 3077 outcome, got %+v", flags)
			}
		},
	)
}
