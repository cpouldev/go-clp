package core

// clp_embed.go vendors the CC-BY EU CLP H/EUH/P statement catalogue and parses it
// into the bilingual base the phrase lookups are built on. The curated entries in
// clp_phrases.go (official Annex III/IV wording + reference-SDS verbatim, marked
// `// ref`) OVERRIDE this base for the codes we have hand-verified; everything
// else — the full P-statement catalogue including combined codes, plus the long
// tail of H/EUH statements — comes from here so the generator is "fully complete".
//
// Source: github.com/mhchem/hpstatements (clp/hpstatements-{el,en}-latest.json),
// itself consolidated from © European Union legal texts (eur-lex.europa.eu) and
// licensed CC BY 4.0, expressly for product labelling. See clpdata/NOTICE.md for
// attribution and refresh instructions.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed clpdata/hpstatements-el-latest.json
var clpDataEL []byte

//go:embed clpdata/hpstatements-en-latest.json
var clpDataEN []byte

// clpDoc is the subset of the mhchem JSON this package reads: the per-code
// statement texts, keyed "latest/<lang>/<code>" (e.g. "latest/en/H302",
// "latest/el/P305+P351+P338"). Each file holds exactly one language.
type clpDoc struct {
	Statements map[string]string `json:"statements"`
}

// codeTextsFromDoc reduces the "latest/<lang>/<code>" keyed statements of one
// single-language file to a plain code→text map. The code is the segment after
// the last '/'; combined codes ("P305+P351+P338") contain no '/', so the prefix
// strip leaves them intact. Panics on malformed embedded data — that is a build
// invariant, not a runtime condition.
func codeTextsFromDoc(raw []byte) map[string]string {
	var doc clpDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		panic(fmt.Sprintf("clp: parse embedded CLP statement data: %v", err))
	}
	out := make(map[string]string, len(doc.Statements))
	for k, v := range doc.Statements {
		code := k
		if i := strings.LastIndex(k, "/"); i >= 0 {
			code = k[i+1:]
		}
		out[code] = v
	}
	return out
}

// mhchemPhrases returns the embedded CLP catalogue as one bilingual phrase per
// code. A code missing a translation in either language is skipped: a half-empty
// phrase would resolve to "" with ok=true and defeat the unknown-code FLAG path
// that routes gaps to manual entry.
func mhchemPhrases() map[string]phrase {
	el := codeTextsFromDoc(clpDataEL)
	en := codeTextsFromDoc(clpDataEN)
	out := make(map[string]phrase, len(en))
	for code, enText := range en {
		elText := el[code]
		if strings.TrimSpace(enText) == "" || strings.TrimSpace(elText) == "" {
			continue
		}
		out[code] = phrase{EL: elText, EN: enText}
	}
	return out
}
