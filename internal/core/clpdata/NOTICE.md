# CLP statement data (vendored)

`hpstatements-el-latest.json` and `hpstatements-en-latest.json` are the Greek and
English CLP hazard (H), supplementary (EUH), and precautionary (P) statement
catalogues, including combined P-codes.

- **Source:** https://github.com/mhchem/hpstatements (`clp/`)
- **Upstream of that data:** consolidated CLP legal texts from https://eur-lex.europa.eu
- **Licence:** © European Union, 1998–2025 — Creative Commons Attribution 4.0
  International (CC BY 4.0). The data set is expressly intended for use in product
  labelling (the licence note states acknowledgement is not required for that use).
- **Content version:** `latest` (see the `_contentVersion` field inside each file).

These files are embedded by `../clp_embed.go` as the *base* catalogue. The curated
entries in `../../../clp_phrases.go` (official Annex III/IV wording + reference-SDS
verbatim, marked `// ref`) **override** this base for codes we have hand-verified.

## Refreshing

```sh
cd api/src/domain/bomsds/clpdata
curl -sSO https://raw.githubusercontent.com/mhchem/hpstatements/master/clp/hpstatements-el-latest.json
curl -sSO https://raw.githubusercontent.com/mhchem/hpstatements/master/clp/hpstatements-en-latest.json
cd .. && go test ./...   # re-run the phrase tests after refreshing
```
