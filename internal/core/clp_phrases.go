package core

// clp_phrases.go is the bilingual (Greek + English) CLP phrase library for the
// EU Safety Data Sheet generator. It is a pure, zero-I/O lookup layer: the
// model authors only CODES into the classification label (H/EUH/P statements,
// hazard-class names, signal word, GHS pictograms) and the print route resolves
// those codes to legally-fixed display text through the functions here.
//
// The Greek text is the OFFICIAL EU CLP Annex III / IV translation. Where a
// code is present in the gold-standard reference SDS the repo ships against
// (LUZI "LUXURY HOTEL 339054", REG-MSDS-EL-01, rev 1.5.0), the Greek string is
// copied VERBATIM from that document; those entries are marked `// ref` below.
// The curated maps in this file are an OVERLAY: the complete H/EUH/P catalogue
// (every code, both languages, incl. combined P-codes) is loaded as a base from
// the embedded CC-BY EU data set (clp_embed.go) in init, then these curated
// entries overwrite it for the codes we have hand-verified. This is legally-fixed
// wording: never paraphrase. A code absent from BOTH the curated maps and the
// embedded catalogue returns ("", false) so the caller can FLAG it for manual
// entry rather than emit a guess.
//
// Lookup contract (shared by every function in this file):
//   - keys are matched case-insensitively and trim-insensitively;
//   - combined P-codes use "+" with no spaces ("P305+P351+P338"); the spaced
//     form is normalised by stripping ALL whitespace, so "P305 + P351 + P338"
//     resolves identically;
//   - for the H/EUH/P lookups an unrecognised Lang falls back to Greek (EL),
//     matching the operator-facing default document language.

import (
	"strings"
)

// phrase is one bilingual statement: the official Greek text and its English
// counterpart. Both fields are always populated for a defined code.
type phrase struct{ EL, EN string }

// normKey upper-cases, trims, and removes ALL internal whitespace from a code
// or key so that lookups tolerate casing, surrounding space, and the spaced
// form of combined P-codes ("P305 + P351 + P338" → "P305+P351+P338").
func normKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			// drop all whitespace
		default:
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

// normHCode normalises a hazard-statement code while PRESERVING the case of its
// differentiation suffix.
//
// That case is legally significant. H360FD is "May damage fertility. May damage
// the unborn child." (Repr. 1 for both endpoints) while H360Fd is "May damage
// fertility. Suspected of damaging the unborn child." (Repr. 1 + Repr. 2) —
// different classifications with different Annex III wording. Folding case
// collapses them onto one map key, and which of the two statements survives is
// then decided by Go's randomised map iteration order, so the same binary prints
// different legal text on different runs.
//
// Only the prefix and digits are case-normalised; everything after the last
// digit is kept verbatim. An ambiguously-cased query ("h360fd") therefore misses
// and is flagged for manual entry rather than resolving to an arbitrary one of
// the two statements.
func normHCode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			// drop all whitespace
		default:
			b.WriteRune(r)
		}
	}
	code := b.String()

	lastDigit := -1
	for i, r := range code {
		if r >= '0' && r <= '9' {
			lastDigit = i
		}
	}
	if lastDigit < 0 {
		return strings.ToUpper(code)
	}
	return strings.ToUpper(code[:lastDigit+1]) + code[lastDigit+1:]
}

// pick returns the requested language's text from a phrase. Any Lang that is
// neither EN nor EL falls back to Greek (the default authoring language).
func (p phrase) pick(lang Lang) string {
	if lang == LangEN {
		return p.EN
	}
	return p.EL
}

// Normalised lookup tables. The source maps below are authored with readable,
// canonically-spelled keys ("Skin Sens. 1B", "P305+P351+P338"); these mirror
// each source map with every key passed through normKey so a query that has
// been normalised the same way matches regardless of casing or whitespace
// (hazard-class names carry internal spaces, so normalising only the query is
// not enough). Built once in init from the source maps.
var (
	hLookup      = map[string]phrase{}
	euhLookup    = map[string]phrase{}
	pLookup      = map[string]phrase{}
	classLookup  = map[string]phrase{}
	signalLookup = map[string]phrase{}
	pictoLookup  = map[string]phrase{}
)

func init() {
	// 1. Base H/EUH/P statements from the embedded CC-BY EU catalogue (full
	//    completeness incl. combined P-codes), split into the three lookups by
	//    code prefix. EUH is tested before H because "EUH" also starts with 'H'
	//    only after the 'E', but a bare HasPrefix(code,"H") would still misroute
	//    "EUHxxx" → check EUH first.
	for code, p := range mhchemPhrases() {
		switch u := strings.ToUpper(code); {
		case strings.HasPrefix(u, "EUH"):
			euhLookup[normHCode(code)] = p
		case strings.HasPrefix(u, "H"):
			hLookup[normHCode(code)] = p
		case strings.HasPrefix(u, "P"):
			pLookup[normKey(code)] = p
		}
	}

	// 2. Curated entries OVERRIDE the base for the codes we have hand-verified
	//    (official Annex III/IV wording + reference-SDS verbatim). Hazard-class
	//    names, signal words, and pictogram alt text are curated-only (no base).
	overlay := func(dst, src map[string]phrase, norm func(string) string) {
		for k, v := range src {
			dst[norm(k)] = v
		}
	}
	overlay(hLookup, hStatements, normHCode)
	overlay(euhLookup, euhStatements, normHCode)
	overlay(pLookup, pStatements, normKey)
	overlay(classLookup, hazardClassNames, normKey)
	overlay(signalLookup, signalWords, normKey)
	overlay(pictoLookup, pictogramDescriptions, normKey)
}

// ── H-statements (CLP Annex III) ──────────────────────────────────────────────
//
// Covers the full fragrance / cosmetic hazard universe: physical (H2xx),
// health (H3xx including the category-specific reproductive-toxicity repr
// variants), and environmental (H4xx). Greek strings are the official Annex III
// translation; entries marked `// ref` are verbatim from the reference SDS.
var hStatements = map[string]phrase{
	// Physical hazards (H2xx).
	"H200": {"Ασταθές εκρηκτικό.", "Unstable explosive."},
	"H201": {"Εκρηκτικό, κίνδυνος μαζικής έκρηξης.", "Explosive; mass explosion hazard."},
	"H202": {"Εκρηκτικό, σοβαρός κίνδυνος εκτόξευσης.", "Explosive; severe projection hazard."},
	"H203": {"Εκρηκτικό. Κίνδυνος πυρκαγιάς, ανατίναξης ή εκτόξευσης.", "Explosive; fire, blast or projection hazard."},
	"H204": {"Κίνδυνος πυρκαγιάς ή εκτόξευσης.", "Fire or projection hazard."},
	"H205": {"Κίνδυνος μαζικής έκρηξης σε περίπτωση πυρκαγιάς.", "May mass explode in fire."},
	"H220": {"Εξαιρετικά εύφλεκτο αέριο.", "Extremely flammable gas."},
	"H221": {"Εύφλεκτο αέριο.", "Flammable gas."},
	"H222": {"Εξαιρετικά εύφλεκτο αερόλυμα.", "Extremely flammable aerosol."},
	"H223": {"Εύφλεκτο αερόλυμα.", "Flammable aerosol."},
	"H224": {"Υγρό και ατμοί εξαιρετικά εύφλεκτα.", "Extremely flammable liquid and vapour."},
	"H225": {"Υγρό και ατμοί πολύ εύφλεκτα.", "Highly flammable liquid and vapour."},
	"H226": {"Υγρό και ατμοί εύφλεκτα.", "Flammable liquid and vapour."},
	"H228": {"Εύφλεκτο στερεό.", "Flammable solid."},
	"H229": {"Δοχείο υπό πίεση. Κατά τη θέρμανση μπορεί να εκραγεί.", "Pressurised container: may burst if heated."},
	// H230-H232 are deliberately NOT curated here: the embedded EU catalogue
	// carries the official Annex III Greek ("Δύναται να εκραγεί …"), and the
	// paraphrases that used to sit here also opened with a Latin "M" (U+004D)
	// instead of the Greek "Μ" (U+039C).
	"H240": {"Η θέρμανση μπορεί να προκαλέσει έκρηξη.", "Heating may cause an explosion."},
	"H241": {"Η θέρμανση μπορεί να προκαλέσει πυρκαγιά ή έκρηξη.", "Heating may cause a fire or explosion."},
	"H242": {"Η θέρμανση μπορεί να προκαλέσει πυρκαγιά.", "Heating may cause a fire."},
	"H250": {"Αυταναφλέγεται εάν εκτεθεί στον αέρα.", "Catches fire spontaneously if exposed to air."},
	"H251": {"Αυτοθερμαίνεται, μπορεί να αναφλεγεί.", "Self-heating; may catch fire."},
	"H252": {
		"Αυτοθερμαίνεται σε μεγάλες ποσότητες, μπορεί να αναφλεγεί.",
		"Self-heating in large quantities; may catch fire.",
	},
	"H260": {
		"Σε επαφή με το νερό ελευθερώνει εύφλεκτα αέρια που μπορούν να αυτοαναφλεγούν.",
		"In contact with water releases flammable gases which may ignite spontaneously.",
	},
	"H261": {"Σε επαφή με το νερό ελευθερώνει εύφλεκτα αέρια.", "In contact with water releases flammable gases."},
	"H270": {"Μπορεί να προκαλέσει ή να αναζωπυρώσει πυρκαγιά, οξειδωτικό.", "May cause or intensify fire; oxidiser."},
	"H271": {
		"Μπορεί να προκαλέσει πυρκαγιά ή έκρηξη, ισχυρό οξειδωτικό.",
		"May cause fire or explosion; strong oxidiser.",
	},
	"H272": {"Μπορεί να αναζωπυρώσει πυρκαγιά, οξειδωτικό.", "May intensify fire; oxidiser."},
	"H280": {
		"Περιέχει αέριο υπό πίεση. Εάν θερμανθεί, μπορεί να εκραγεί.",
		"Contains gas under pressure; may explode if heated.",
	},
	"H281": {
		"Περιέχει βαθύψυκτο αέριο. Μπορεί να προκαλέσει εγκαύματα ή τραυματισμούς από ψύχος.",
		"Contains refrigerated gas; may cause cryogenic burns or injury.",
	},
	"H290": {"Μπορεί να διαβρώσει μέταλλα.", "May be corrosive to metals."},

	// Health hazards (H3xx).
	"H300": {"Θανατηφόρο σε περίπτωση κατάποσης.", "Fatal if swallowed."},
	"H301": {"Τοξικό σε περίπτωση κατάποσης.", "Toxic if swallowed."},
	"H302": {"Επιβλαβές σε περίπτωση κατάποσης.", "Harmful if swallowed."}, // ref
	"H304": {
		"Μπορεί να προκαλέσει θάνατο σε περίπτωση κατάποσης και διείσδυσης στις αναπνευστικές οδούς.",
		"May be fatal if swallowed and enters airways.",
	}, // ref
	"H310": {"Θανατηφόρο σε επαφή με το δέρμα.", "Fatal in contact with skin."},
	"H311": {"Τοξικό σε επαφή με το δέρμα.", "Toxic in contact with skin."},
	"H312": {"Επιβλαβές σε επαφή με το δέρμα.", "Harmful in contact with skin."},
	"H314": {
		"Προκαλεί σοβαρά δερματικά εγκαύματα και οφθαλμικές βλάβες.",
		"Causes severe skin burns and eye damage.",
	}, // ref
	"H315": {"Προκαλεί ερεθισμό του δέρματος.", "Causes skin irritation."},                                  // ref
	"H317": {"Μπορεί να προκαλέσει αλλεργική δερματική αντίδραση.", "May cause an allergic skin reaction."}, // ref
	"H318": {"Προκαλεί σοβαρή οφθαλμική βλάβη.", "Causes serious eye damage."},                              // ref
	"H319": {"Προκαλεί σοβαρό οφθαλμικό ερεθισμό.", "Causes serious eye irritation."},                       // ref
	"H330": {"Θανατηφόρο σε περίπτωση εισπνοής.", "Fatal if inhaled."},
	"H331": {"Τοξικό σε περίπτωση εισπνοής.", "Toxic if inhaled."},
	"H332": {"Επιβλαβές σε περίπτωση εισπνοής.", "Harmful if inhaled."},
	"H334": {
		"Μπορεί να προκαλέσει αλλεργία ή συμπτώματα άσθματος ή δύσπνοια σε περίπτωση εισπνοής.",
		"May cause allergy or asthma symptoms or breathing difficulties if inhaled.",
	},
	"H335":  {"Μπορεί να προκαλέσει ερεθισμό της αναπνευστικής οδού.", "May cause respiratory irritation."},
	"H336":  {"Μπορεί να προκαλέσει υπνηλία ή ζάλη.", "May cause drowsiness or dizziness."},
	"H340":  {"Μπορεί να προκαλέσει γενετικά ελαττώματα.", "May cause genetic defects."},
	"H341":  {"Ύποπτο για πρόκληση γενετικών ελαττωμάτων.", "Suspected of causing genetic defects."},
	"H350":  {"Μπορεί να προκαλέσει καρκίνο.", "May cause cancer."},
	"H350i": {"Μπορεί να προκαλέσει καρκίνο σε περίπτωση εισπνοής.", "May cause cancer if inhaled."},
	"H351":  {"Ύποπτο για πρόκληση καρκίνου.", "Suspected of causing cancer."},
	"H360":  {"Μπορεί να βλάψει τη γονιμότητα ή το έμβρυο.", "May damage fertility or the unborn child."},
	"H360F": {"Μπορεί να βλάψει τη γονιμότητα.", "May damage fertility."},
	"H360D": {"Μπορεί να βλάψει το έμβρυο.", "May damage the unborn child."},
	"H360FD": {
		"Μπορεί να βλάψει τη γονιμότητα. Μπορεί να βλάψει το έμβρυο.",
		"May damage fertility. May damage the unborn child.",
	},
	"H360Fd": {
		"Μπορεί να βλάψει τη γονιμότητα. Ύποπτο για πρόκληση βλάβης στο έμβρυο.",
		"May damage fertility. Suspected of damaging the unborn child.",
	},
	"H360Df": {
		"Μπορεί να βλάψει το έμβρυο. Ύποπτο για πρόκληση βλάβης στη γονιμότητα.",
		"May damage the unborn child. Suspected of damaging fertility.",
	},
	"H361": {
		"Ύποπτο για πρόκληση βλάβης στη γονιμότητα ή στο έμβρυο.",
		"Suspected of damaging fertility or the unborn child.",
	},
	"H361f": {"Ύποπτο για πρόκληση βλάβης στη γονιμότητα.", "Suspected of damaging fertility."},
	"H361d": {"Ύποπτο για πρόκληση βλάβης στο έμβρυο.", "Suspected of damaging the unborn child."},
	"H361fd": {
		"Ύποπτο για πρόκληση βλάβης στη γονιμότητα. Ύποπτο για πρόκληση βλάβης στο έμβρυο.",
		"Suspected of damaging fertility. Suspected of damaging the unborn child.",
	},
	"H362": {"Μπορεί να βλάψει τα βρέφη που τρέφονται με μητρικό γάλα.", "May cause harm to breast-fed children."},
	"H370": {"Προκαλεί βλάβες στα όργανα.", "Causes damage to organs."},
	"H371": {"Μπορεί να προκαλέσει βλάβες στα όργανα.", "May cause damage to organs."},
	"H372": {
		"Προκαλεί βλάβες στα όργανα ύστερα από παρατεταμένη ή επανειλημμένη έκθεση.",
		"Causes damage to organs through prolonged or repeated exposure.",
	},
	"H373": {
		"Μπορεί να προκαλέσει βλάβες στα όργανα ύστερα από παρατεταμένη ή επανειλημμένη έκθεση.",
		"May cause damage to organs through prolonged or repeated exposure.",
	},

	// Environmental hazards (H4xx).
	"H400": {"Πολύ τοξικό για τους υδρόβιους οργανισμούς.", "Very toxic to aquatic life."},
	"H410": {
		"Πολύ τοξικό για τους υδρόβιους οργανισμούς, με μακροχρόνιες επιπτώσεις.",
		"Very toxic to aquatic life with long lasting effects.",
	}, // ref
	"H411": {
		"Τοξικό για τους υδρόβιους οργανισμούς, με μακροχρόνιες επιπτώσεις.",
		"Toxic to aquatic life with long lasting effects.",
	}, // ref
	"H412": {
		"Επιβλαβές για τους υδρόβιους οργανισμούς, με μακροχρόνιες επιπτώσεις.",
		"Harmful to aquatic life with long lasting effects.",
	}, // ref
	"H413": {
		"Μπορεί να προκαλέσει μακροχρόνιες επιπτώσεις στους υδρόβιους οργανισμούς.",
		"May cause long lasting harmful effects to aquatic life.",
	},
	"H420": {
		"Βλάπτει τη δημόσια υγεία και το περιβάλλον καταστρέφοντας το όζον στην ανώτερη ατμόσφαιρα.",
		"Harms public health and the environment by destroying ozone in the upper atmosphere.",
	},
}

// ── EUH supplementary statements (CLP Annex II / III) ─────────────────────────
var euhStatements = map[string]phrase{
	"EUH014": {"Αντιδρά βίαια με το νερό.", "Reacts violently with water."},
	"EUH018": {
		"Κατά τη χρήση μπορεί να σχηματίσει εύφλεκτα ή εκρηκτικά μείγματα ατμού-αέρος.",
		"In use may form flammable/explosive vapour-air mixture.",
	},
	"EUH019": {"Μπορεί να σχηματίσει εκρηκτικά υπεροξείδια.", "May form explosive peroxides."},
	"EUH029": {"Σε επαφή με το νερό ελευθερώνει τοξικά αέρια.", "Contact with water liberates toxic gas."},
	"EUH031": {"Σε επαφή με οξέα ελευθερώνει τοξικό αέριο.", "Contact with acids liberates toxic gas."},
	"EUH032": {"Σε επαφή με οξέα ελευθερώνει πολύ τοξικό αέριο.", "Contact with acids liberates very toxic gas."},
	"EUH044": {"Κίνδυνος εκρήξεως εάν θερμανθεί υπό περιορισμό.", "Risk of explosion if heated under confinement."},
	"EUH059": {"Επικίνδυνο για τη στιβάδα του όζοντος.", "Hazardous to the ozone layer."},
	"EUH066": {
		"Παρατεταμένη έκθεση μπορεί να προκαλέσει ξηρότητα δέρματος ή σκάσιμο.",
		"Repeated exposure may cause skin dryness or cracking.",
	},
	"EUH070": {"Τοξικό σε επαφή με τα μάτια.", "Toxic by eye contact."},
	"EUH071": {"Διαβρωτικό της αναπνευστικής οδού.", "Corrosive to the respiratory tract."},
	"EUH201": {
		"Περιέχει μόλυβδο. Να μη χρησιμοποιείται σε επιφάνειες που είναι πιθανό να μασηθούν ή να πιπιλιστούν από παιδιά.",
		"Contains lead. Should not be used on surfaces liable to be chewed or sucked by children.",
	},
	"EUH204": {
		"Περιέχει ισοκυανικές ενώσεις. Μπορεί να προκαλέσει αλλεργική αντίδραση.",
		"Contains isocyanates. May produce an allergic reaction.",
	},
	"EUH205": {
		"Περιέχει εποξειδικές ενώσεις. Μπορεί να προκαλέσει αλλεργική αντίδραση.",
		"Contains epoxy constituents. May produce an allergic reaction.",
	},
	// EUH208 carries a substance name suffix on the SDS label; the canonical
	// Annex II text is the lead-in clause "Περιέχει <name>. ...". The reference
	// SDS prints the header "Περιέχει ένα συστατικό που μπορεί να προκαλέσει
	// αλλεργική αντίδραση." then lists the components; that header is reproduced
	// verbatim here as the standard generic text.
	"EUH208": {
		"Περιέχει ένα συστατικό που μπορεί να προκαλέσει αλλεργική αντίδραση.",
		"Contains a component that may produce an allergic reaction.",
	}, // ref (header)
	"EUH209": {"Μπορεί να καταστεί πολύ εύφλεκτο κατά τη χρήση.", "Can become highly flammable in use."},
	"EUH210": {"Δελτίο δεδομένων ασφαλείας παρέχεται εφόσον ζητηθεί.", "Safety data sheet available on request."},
	"EUH211": {
		"Προσοχή! Κατά τον ψεκασμό μπορούν να σχηματιστούν επικίνδυνα εισπνεύσιμα σταγονίδια. Μην αναπνέετε το εκνέφωμα ή τα σταγονίδια.",
		"Warning! Hazardous respirable droplets may be formed when sprayed. Do not breathe spray or mist.",
	},
	"EUH212": {
		"Προσοχή! Κατά τη χρήση μπορεί να σχηματιστεί επικίνδυνη εισπνεύσιμη σκόνη. Μην αναπνέετε τη σκόνη.",
		"Warning! Hazardous respirable dust may be formed when used. Do not breathe dust.",
	},
	"EUH401": {
		"Για να αποφύγετε τους κινδύνους για την ανθρώπινη υγεία και το περιβάλλον, ακολουθήστε τις οδηγίες χρήσης.",
		"To avoid risks to human health and the environment, comply with the instructions for use.",
	},
}

// ── P-statements (CLP Annex IV), including the common combined codes ──────────
//
// Combined codes are keyed exactly as printed on labels ("P305+P351+P338").
// Where the published statement carries bracketed manufacturer options
// ("[or doctor]", "...specified by the manufacturer..."), a sensible filled-in
// default is given. Entries marked `// ref` are verbatim from the reference SDS.
var pStatements = map[string]phrase{
	// General (P1xx).
	"P101": {
		"Εάν ζητήσετε ιατρική συμβουλή, να έχετε μαζί σας τον περιέκτη του προϊόντος ή την ετικέτα.",
		"If medical advice is needed, have product container or label at hand.",
	}, // ref
	"P102": {"Μακριά από παιδιά.", "Keep out of reach of children."},
	"P103": {"Διαβάστε την ετικέτα πριν από τη χρήση.", "Read label before use."}, // ref

	// Prevention (P2xx).
	"P201": {"Εφοδιαστείτε με τις ειδικές οδηγίες πριν από τη χρήση.", "Obtain special instructions before use."},
	"P202": {
		"Μην το χρησιμοποιήσετε πριν διαβάσετε και κατανοήσετε τις οδηγίες προφύλαξης.",
		"Do not handle until all safety precautions have been read and understood.",
	},
	"P210": {
		"Μακριά από θερμότητα, θερμές επιφάνειες, σπινθήρες, γυμνές φλόγες και άλλες πηγές ανάφλεξης. Μην καπνίζετε.",
		"Keep away from heat, hot surfaces, sparks, open flames and other ignition sources. No smoking.",
	},
	"P211": {
		"Μην ψεκάζετε κοντά σε γυμνή φλόγα ή άλλη πηγή ανάφλεξης.",
		"Do not spray on an open flame or other ignition source.",
	},
	"P220": {
		"Φυλάσσεται μακριά από ρουχισμό και άλλα καύσιμα υλικά.",
		"Keep away from clothing and other combustible materials.",
	},
	"P221": {
		"Λάβετε κάθε προφύλαξη ώστε να μην αναμειχθεί με καύσιμα.",
		"Take any precaution to avoid mixing with combustibles.",
	},
	"P222": {"Να μην έρθει σε επαφή με τον αέρα.", "Do not allow contact with air."},
	"P223": {"Να μην έρθει σε επαφή με το νερό.", "Do not allow contact with water."},
	"P231": {
		"Ο χειρισμός και η αποθήκευση του περιεχομένου να γίνεται υπό αδρανές αέριο.",
		"Handle and store contents under inert gas.",
	},
	"P232": {"Προστατέψτε από την υγρασία.", "Protect from moisture."},
	"P233": {"Διατηρείται ο περιέκτης ερμητικά κλειστός.", "Keep container tightly closed."},
	"P234": {"Να φυλάσσεται μόνο στον αρχικό περιέκτη.", "Keep only in original packaging."},
	"P235": {"Διατηρείται δροσερό.", "Keep cool."},
	"P240": {
		"Γειώστε και συνδέστε τον περιέκτη και τον δέκτη του υλικού.",
		"Ground and bond container and receiving equipment.",
	},
	"P241": {
		"Να χρησιμοποιείται αντιεκρηκτικός ηλεκτρολογικός εξοπλισμός, εξοπλισμός εξαερισμού και φωτισμού.",
		"Use explosion-proof electrical, ventilating and lighting equipment.",
	},
	"P242": {"Να χρησιμοποιούνται μη σπινθηρογόνα εργαλεία.", "Use non-sparking tools."},
	"P243": {
		"Λάβετε προστατευτικά μέτρα έναντι ηλεκτροστατικών εκκενώσεων.",
		"Take action to prevent static discharges.",
	},
	"P244": {
		"Διατηρείτε τα κλείστρα και τα ρακόρ καθαρά από λάδια και γράσα.",
		"Keep valves and fittings free from oil and grease.",
	},
	// P250 is deliberately NOT curated here: the embedded EU catalogue carries the
	// official Annex IV Greek ("Να αποφεύγεται άλεση/κρούση/τριβή/… ."). The
	// paraphrase that used to sit here read "τριβή … ή τριβή" — friction twice,
	// with grinding missing altogether.
	"P251": {"Να μην τρυπηθεί ή καεί ακόμη και μετά τη χρήση.", "Do not pierce or burn, even after use."},
	"P260": {
		"Μην αναπνέετε σκόνη/αναθυμιάσεις/αέρια/σταγονίδια/ατμούς/εκνεφώματα.",
		"Do not breathe dust/fume/gas/mist/vapours/spray.",
	},
	"P261": {
		"Αποφεύγετε να αναπνέετε σκόνη/αναθυμιάσεις/αέρια/σταγονίδια/ατμούς/εκνεφώματα.",
		"Avoid breathing dust/fume/gas/mist/vapours/spray.",
	}, // ref
	"P262": {
		"Να μην έλθει σε επαφή με τα μάτια, με το δέρμα ή με τα ρούχα.",
		"Do not get in eyes, on skin, or on clothing.",
	},
	"P263": {
		"Αποφεύγετε την επαφή κατά τη διάρκεια της εγκυμοσύνης και της γαλουχίας.",
		"Avoid contact during pregnancy and while nursing.",
	},
	"P264": {"Πλύνετε χέρια σχολαστικά μετά το χειρισμό.", "Wash hands thoroughly after handling."}, // ref
	"P270": {
		"Μην τρώτε, πίνετε ή καπνίζετε όταν χρησιμοποιείτε αυτό το προϊόν.",
		"Do not eat, drink or smoke when using this product.",
	},
	"P271": {
		"Να χρησιμοποιείται μόνο σε ανοιχτό ή καλά αεριζόμενο χώρο.",
		"Use only outdoors or in a well-ventilated area.",
	},
	"P272": {
		"Τα μολυσμένα ενδύματα εργασίας δεν πρέπει να βγαίνουν από το χώρο εργασίας.",
		"Contaminated work clothing should not be allowed out of the workplace.",
	}, // ref
	"P273": {"Να αποφεύγεται η ελευθέρωση στο περιβάλλον.", "Avoid release to the environment."}, // ref
	"P280": {
		"Να φοράτε προστατευτικά γάντια/προστατευτικά ενδύματα/μέσα ατομικής προστασίας για τα μάτια/πρόσωπο.",
		"Wear protective gloves/protective clothing/eye protection/face protection.",
	}, // ref

	// Response (P3xx).
	"P301": {"ΣΕ ΠΕΡΙΠΤΩΣΗ ΚΑΤΑΠΟΣΗΣ:", "IF SWALLOWED:"},
	"P301+P310": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΚΑΤΑΠΟΣΗΣ: Καλέστε αμέσως το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ ή έναν γιατρό.",
		"IF SWALLOWED: Immediately call a POISON CENTER or doctor.",
	},
	"P301+P312": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΚΑΤΑΠΟΣΗΣ: Καλέστε το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ ή έναν γιατρό εάν αισθανθείτε αδιαθεσία.",
		"IF SWALLOWED: Call a POISON CENTER or doctor if you feel unwell.",
	},
	"P301+P330+P331": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΚΑΤΑΠΟΣΗΣ: Ξεπλύνετε το στόμα. ΜΗΝ προκαλέσετε εμετό.",
		"IF SWALLOWED: Rinse mouth. Do NOT induce vomiting.",
	},
	"P302": {"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΟ ΔΕΡΜΑ:", "IF ON SKIN:"},
	"P302+P352": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΟ ΔΕΡΜΑ: Πλύντε με άφθονο νερό και σαπούνι.",
		"IF ON SKIN: Wash with plenty of water and soap.",
	}, // ref
	"P303": {"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΟ ΔΕΡΜΑ (ή με τα μαλλιά):", "IF ON SKIN (or hair):"},
	"P303+P361+P353": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΟ ΔΕΡΜΑ (ή με τα μαλλιά): Αφαιρέστε αμέσως όλα τα μολυσμένα ενδύματα. Ξεπλύνετε την επιδερμίδα με νερό ή στο ντους.",
		"IF ON SKIN (or hair): Take off immediately all contaminated clothing. Rinse skin with water or shower.",
	},
	"P304": {"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΙΣΠΝΟΗΣ:", "IF INHALED:"},
	"P304+P340": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΙΣΠΝΟΗΣ: Μεταφέρετε τον παθόντα στον καθαρό αέρα και αφήστε τον να ξεκουραστεί σε στάση που διευκολύνει την αναπνοή.",
		"IF INHALED: Remove person to fresh air and keep comfortable for breathing.",
	},
	"P305": {"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΑ ΜΑΤΙΑ:", "IF IN EYES:"},
	"P305+P351+P338": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ ΕΠΑΦΗΣ ΜΕ ΤΑ ΜΑΤΙΑ: Ξεπλύνετε προσεκτικά με νερό για αρκετά λεπτά. Αν υπάρχουν φακοί επαφής, αφαιρέστε τους, αν είναι εύκολο. Συνεχίστε να ξεπλένετε.",
		"IF IN EYES: Rinse cautiously with water for several minutes. Remove contact lenses, if present and easy to do. Continue rinsing.",
	}, // ref
	"P308": {"ΣΕ ΠΕΡΙΠΤΩΣΗ έκθεσης ή πιθανής έκθεσης:", "IF exposed or concerned:"},
	"P308+P311": {
		"ΣΕ ΠΕΡΙΠΤΩΣΗ έκθεσης ή πιθανής έκθεσης: Καλέστε το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ ή έναν γιατρό.",
		"IF exposed or concerned: Call a POISON CENTER or doctor.",
	},
	"P310": {
		"Καλέστε αμέσως το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ ή έναν γιατρό.",
		"Immediately call a POISON CENTER or doctor.",
	},
	"P311": {"Καλέστε το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ ή έναν γιατρό.", "Call a POISON CENTER or doctor."},
	"P312": {
		"Καλέστε το ΚΕΝΤΡΟ ΔΗΛΗΤΗΡΙΑΣΕΩΝ ή έναν γιατρό εάν αισθανθείτε αδιαθεσία.",
		"Call a POISON CENTER or doctor if you feel unwell.",
	},
	"P321": {
		"Χρειάζεται ειδική αγωγή (βλέπε ... στην ετικέτα).",
		"Specific treatment (see ... on this label).",
	}, // ref
	"P330": {"Ξεπλύνετε το στόμα.", "Rinse mouth."},
	"P331": {"ΜΗΝ προκαλέσετε εμετό.", "Do NOT induce vomiting."},
	"P332": {"Εάν παρατηρηθεί ερεθισμός του δέρματος:", "If skin irritation occurs:"},
	"P332+P313": {
		"Εάν παρατηρηθεί ερεθισμός του δέρματος: Συμβουλευθείτε/Επισκεφθείτε γιατρό.",
		"If skin irritation occurs: Get medical advice/attention.",
	}, // ref
	"P333": {
		"Εάν παρατηρηθεί ερεθισμός του δέρματος ή εμφανιστεί εξάνθημα:",
		"If skin irritation or rash occurs:",
	},
	"P333+P313": {
		"Εάν παρατηρηθεί ερεθισμός του δέρματος ή εμφανιστεί εξάνθημα: Συμβουλευθείτε/Επισκεφθείτε γιατρό.",
		"If skin irritation or rash occurs: Get medical advice/attention.",
	}, // ref
	"P337": {"Εάν δεν υποχωρεί ο οφθαλμικός ερεθισμός:", "If eye irritation persists:"},
	"P337+P313": {
		"Εάν δεν υποχωρεί ο οφθαλμικός ερεθισμός: Συμβουλευθείτε/Επισκεφθείτε γιατρό.",
		"If eye irritation persists: Get medical advice/attention.",
	}, // ref
	"P362": {"Βγάλτε τα μολυσμένα ρούχα.", "Take off contaminated clothing."}, // ref
	"P362+P364": {
		"Βγάλτε τα μολυσμένα ρούχα και πλύντε τα πριν τα ξαναχρησιμοποιήσετε.",
		"Take off contaminated clothing and wash it before reuse.",
	},
	"P363": {
		"Πλύνετε τα μολυσμένα ενδύματα πριν τα ξαναχρησιμοποιήσετε.",
		"Wash contaminated clothing before reuse.",
	}, // ref
	"P370": {"Σε περίπτωση πυρκαγιάς:", "In case of fire:"},
	"P370+P378": {
		"Σε περίπτωση πυρκαγιάς: Χρησιμοποιήστε ξηρή άμμο, ξηρή χημική σκόνη ή ανθεκτικό στην αλκοόλη αφρό για την κατάσβεση.",
		"In case of fire: Use dry sand, dry chemical or alcohol-resistant foam to extinguish.",
	},
	"P391": {"Μαζέψτε τη χυμένη ποσότητα.", "Collect spillage."}, // ref

	// Storage (P4xx) and disposal (P5xx).
	"P401": {"Αποθηκεύεται σύμφωνα με τους τοπικούς κανονισμούς.", "Store in accordance with local regulations."},
	"P402": {"Αποθηκεύεται σε ξηρό μέρος.", "Store in a dry place."},
	"P403": {"Αποθηκεύεται σε καλά αεριζόμενο χώρο.", "Store in a well-ventilated place."},
	"P403+P233": {
		"Αποθηκεύεται σε καλά αεριζόμενο χώρο. Ο περιέκτης διατηρείται ερμητικά κλειστός.",
		"Store in a well-ventilated place. Keep container tightly closed.",
	},
	"P403+P235": {
		"Αποθηκεύεται σε καλά αεριζόμενο χώρο. Διατηρείται δροσερό.",
		"Store in a well-ventilated place. Keep cool.",
	},
	"P405": {"Φυλάσσεται κλειδωμένο.", "Store locked up."},
	"P410": {"Να προστατεύεται από τις ηλιακές ακτίνες.", "Protect from sunlight."},
	"P411": {
		"Αποθηκεύεται σε θερμοκρασία που δεν υπερβαίνει τους 50 °C.",
		"Store at temperatures not exceeding 50 °C.",
	},
	"P412": {
		"Δεν εκτίθεται σε θερμοκρασίες που υπερβαίνουν τους 50 °C.",
		"Do not expose to temperatures exceeding 50 °C.",
	},
	"P420": {"Αποθηκεύεται μακριά από άλλα υλικά.", "Store away from other materials."},
	"P501": {
		"Απορρίψτε το περιεχόμενο/περιέκτη σύμφωνα με τους επίσημους κανονισμούς.",
		"Dispose of contents/container in accordance with local regulations.",
	}, // ref
}

// ── Hazard-class display names ────────────────────────────────────────────────
//
// Keys mirror the abbreviated forms the classifier and prompt use (e.g.
// "Skin Sens. 1", "Skin Sens. 1B", "Aquatic Chronic 2", "Acute Tox. 4 oral").
// Both the bare-category and category-suffixed variants are registered where
// the SDS prints both. Greek strings come from the reference SDS Section 16.2
// (verbatim where present) and the CLP Annex VI class taxonomy.
var hazardClassNames = map[string]phrase{
	// Acute toxicity (route variants printed on the SDS).
	"Acute Tox. 1": {"Οξεία τοξικότητα 1", "Acute toxicity 1"},
	"Acute Tox. 2": {"Οξεία τοξικότητα 2", "Acute toxicity 2"},
	"Acute Tox. 3": {"Οξεία τοξικότητα 3", "Acute toxicity 3"},
	"Acute Tox. 4": {"Οξεία τοξικότητα 4", "Acute toxicity 4"},
	"Acute Tox. 4 oral": {
		"Οξεία τοξικότητα 4 (από του στόματος)", "Acute toxicity 4 (oral)",
	}, // ref (Section 16.2: "οξεία τοξικότητα από του στόματος")
	"Acute Tox. 4 dermal":     {"Οξεία τοξικότητα 4 (δερματική)", "Acute toxicity 4 (dermal)"},
	"Acute Tox. 4 inhalation": {"Οξεία τοξικότητα 4 (εισπνοή)", "Acute toxicity 4 (inhalation)"},

	// Skin / eye / sensitisation.
	"Skin Corr. 1":  {"Διάβρωση του δέρματος 1", "Skin corrosion 1"},
	"Skin Corr. 1A": {"Διάβρωση του δέρματος 1A", "Skin corrosion 1A"},
	"Skin Corr. 1B": {"Διάβρωση του δέρματος 1B", "Skin corrosion 1B"}, // ref
	"Skin Corr. 1C": {"Διάβρωση του δέρματος 1C", "Skin corrosion 1C"},
	"Skin Irrit. 2": {"Ερεθισμός του δέρματος 2", "Skin irritation 2"},    // ref
	"Eye Dam. 1":    {"Σοβαρή οφθαλμική βλάβη 1", "Serious eye damage 1"}, // ref
	"Eye Irrit. 2":  {"Ερεθισμός των οφθαλμών 2", "Eye irritation 2"},     // ref
	"Skin Sens. 1":  {"Ευαισθητοποίηση του δέρματος 1", "Skin sensitisation 1"},
	"Skin Sens. 1A": {"Ευαισθητοποίηση του δέρματος 1A", "Skin sensitisation 1A"},
	"Skin Sens. 1B": {"Ευαισθητοποίηση του δέρματος 1B", "Skin sensitisation 1B"}, // ref
	"Resp. Sens. 1": {"Ευαισθητοποίηση του αναπνευστικού 1", "Respiratory sensitisation 1"},

	// CMR + systemic.
	"Carc. 1A": {"Καρκινογένεση 1A", "Carcinogenicity 1A"},
	"Carc. 1B": {"Καρκινογένεση 1B", "Carcinogenicity 1B"},
	"Carc. 2":  {"Καρκινογένεση 2", "Carcinogenicity 2"},
	"Muta. 1A": {"Μεταλλαξιγένεση γεννητικών κυττάρων 1A", "Germ cell mutagenicity 1A"},
	"Muta. 1B": {"Μεταλλαξιγένεση γεννητικών κυττάρων 1B", "Germ cell mutagenicity 1B"},
	"Muta. 2":  {"Μεταλλαξιγένεση γεννητικών κυττάρων 2", "Germ cell mutagenicity 2"},
	"Repr. 1A": {"Αναπαραγωγική τοξικότητα 1A", "Reproductive toxicity 1A"},
	"Repr. 1B": {"Αναπαραγωγική τοξικότητα 1B", "Reproductive toxicity 1B"},
	"Repr. 2":  {"Αναπαραγωγική τοξικότητα 2", "Reproductive toxicity 2"},
	"Lact.":    {"Επιδράσεις στο ή μέσω του θηλασμού", "Effects on or via lactation"},
	"STOT SE 1": {
		"Ειδική τοξικότητα στα όργανα-στόχους (εφάπαξ έκθεση) 1",
		"Specific target organ toxicity (single exposure) 1",
	},
	"STOT SE 2": {
		"Ειδική τοξικότητα στα όργανα-στόχους (εφάπαξ έκθεση) 2",
		"Specific target organ toxicity (single exposure) 2",
	},
	"STOT SE 3": {
		"Ειδική τοξικότητα στα όργανα-στόχους (εφάπαξ έκθεση) 3",
		"Specific target organ toxicity (single exposure) 3",
	},
	"STOT RE 1": {
		"Ειδική τοξικότητα στα όργανα-στόχους (επανειλημμένη έκθεση) 1",
		"Specific target organ toxicity (repeated exposure) 1",
	},
	"STOT RE 2": {
		"Ειδική τοξικότητα στα όργανα-στόχους (επανειλημμένη έκθεση) 2",
		"Specific target organ toxicity (repeated exposure) 2",
	},
	"Asp. Tox. 1": {"Κίνδυνος αναρρόφησης 1", "Aspiration hazard 1"}, // ref (Section 16.2: "Κίνδυνος από αναρρόφηση")

	// Physical.
	"Flam. Liq. 1": {"Εύφλεκτο υγρό 1", "Flammable liquid 1"},
	"Flam. Liq. 2": {"Εύφλεκτο υγρό 2", "Flammable liquid 2"},
	"Flam. Liq. 3": {"Εύφλεκτο υγρό 3", "Flammable liquid 3"},
	"Flam. Sol. 1": {"Εύφλεκτο στερεό 1", "Flammable solid 1"},
	"Flam. Sol. 2": {"Εύφλεκτο στερεό 2", "Flammable solid 2"},
	"Aerosol 1":    {"Αερόλυμα 1", "Aerosol 1"},
	"Aerosol 2":    {"Αερόλυμα 2", "Aerosol 2"},
	"Aerosol 3":    {"Αερόλυμα 3", "Aerosol 3"},
	"Met. Corr. 1": {"Διαβρωτικό για τα μέταλλα 1", "Corrosive to metals 1"},

	// Environmental.
	"Aquatic Acute 1": {
		"Επικίνδυνο για το υδάτινο περιβάλλον - Οξύς κίνδυνος 1",
		"Hazardous to the aquatic environment - Acute 1",
	},
	"Aquatic Chronic 1": {
		"Επικίνδυνο για το υδάτινο περιβάλλον - Χρόνιος κίνδυνος 1",
		"Hazardous to the aquatic environment - Chronic 1",
	}, // ref (Section 16.2)
	"Aquatic Chronic 2": {
		"Επικίνδυνο για το υδάτινο περιβάλλον - Χρόνιος κίνδυνος 2",
		"Hazardous to the aquatic environment - Chronic 2",
	}, // ref
	"Aquatic Chronic 3": {
		"Επικίνδυνο για το υδάτινο περιβάλλον - Χρόνιος κίνδυνος 3",
		"Hazardous to the aquatic environment - Chronic 3",
	}, // ref
	"Aquatic Chronic 4": {
		"Επικίνδυνο για το υδάτινο περιβάλλον - Χρόνιος κίνδυνος 4",
		"Hazardous to the aquatic environment - Chronic 4",
	},
	"Ozone 1": {"Επικίνδυνο για τη στιβάδα του όζοντος 1", "Hazardous to the ozone layer 1"},
}

// ── Hazard-class family by H-code ─────────────────────────────────────────────
//
// hCodeFamily maps every standard CLP H-statement code to its hazard-class
// FAMILY abbreviation (the class WITHOUT a category — the category lives on the
// supplier-extracted hazard so its 1A/1B precision is preserved). It is used to
// attach each H-code to the right §3 classification entry: a supplier extraction
// often repeats a substance's full code list on every hazard row, and matching
// codes to their family collapses that to one code per class. Acute Tox. and
// Skin Corr. share a family across categories by design (category comes from the
// extraction, not the code). Suffixed codes (H360FD, H350i, H361fd) resolve via
// their three-digit base in hCodeBase.
var hCodeFamily = map[string]string{
	// Acute toxicity — all routes and categories share the family.
	"H300": "Acute Tox.", "H301": "Acute Tox.", "H302": "Acute Tox.",
	"H310": "Acute Tox.", "H311": "Acute Tox.", "H312": "Acute Tox.",
	"H330": "Acute Tox.", "H331": "Acute Tox.", "H332": "Acute Tox.",
	// Skin / eye / sensitisation.
	"H314": "Skin Corr.", "H315": "Skin Irrit.",
	"H318": "Eye Dam.", "H319": "Eye Irrit.",
	"H317": "Skin Sens.", "H334": "Resp. Sens.",
	// CMR + lactation.
	"H340": "Muta.", "H341": "Muta.",
	"H350": "Carc.", "H351": "Carc.",
	"H360": "Repr.", "H361": "Repr.", "H362": "Lact.",
	// STOT (single / repeated exposure).
	"H335": "STOT SE", "H336": "STOT SE", "H370": "STOT SE", "H371": "STOT SE",
	"H372": "STOT RE", "H373": "STOT RE",
	// Aspiration.
	"H304": "Asp. Tox.",
	// Physical (the classes a fragrance mixture realistically carries).
	"H224": "Flam. Liq.", "H225": "Flam. Liq.", "H226": "Flam. Liq.",
	"H228": "Flam. Sol.", "H290": "Met. Corr.",
	// Environmental.
	"H400": "Aquatic Acute", "H410": "Aquatic Chronic", "H411": "Aquatic Chronic",
	"H412": "Aquatic Chronic", "H413": "Aquatic Chronic", "H420": "Ozone",
}

// hCodeBase reduces an H-code to its three-digit base form ("H361fd" → "H361",
// "H350i" → "H350", "h226" → "H226"), uppercasing and dropping any whitespace and
// trailing sub-classification letters. Returns the input uppercased/trimmed when
// it does not look like an H-code.
func hCodeBase(code string) string {
	s := strings.ToUpper(strings.TrimSpace(code))
	s = strings.ReplaceAll(s, " ", "")
	if !strings.HasPrefix(s, "H") {
		return s
	}
	n := 1
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return s[:n]
}

// familyForHCode returns the hazard-class family abbreviation for an H-code, or
// ("", false) when the code is unknown (the caller then keeps the code as-is).
func familyForHCode(code string) (string, bool) {
	fam, ok := hCodeFamily[hCodeBase(code)]
	return fam, ok
}

// ── Signal words (CLP Annex I §1.4) ───────────────────────────────────────────
var signalWords = map[string]phrase{
	"DANGER":  {"Κίνδυνος", "Danger"},
	"WARNING": {"Προσοχή", "Warning"}, // ref (Section 2.2.2: "Προσοχή")
}

// ── GHS pictogram alt text (CLP Annex V) ──────────────────────────────────────
var pictogramDescriptions = map[string]phrase{
	"GHS01": {"Εκρηγνυόμενη βόμβα", "Exploding bomb"},
	"GHS02": {"Φλόγα", "Flame"},
	"GHS03": {"Φλόγα πάνω από κύκλο", "Flame over circle"},
	"GHS04": {"Φιάλη αερίου", "Gas cylinder"},
	"GHS05": {"Διάβρωση", "Corrosion"},
	"GHS06": {"Νεκροκεφαλή με χιαστί οστά", "Skull and crossbones"},
	"GHS07": {"Θαυμαστικό", "Exclamation mark"}, // ref (Section 2.1.1: "GHS07 Επιβλαβές", exclamation-mark pictogram)
	"GHS08": {"Κίνδυνος για την υγεία", "Health hazard"},
	"GHS09": {"Περιβάλλον", "Environment"}, // ref (Section 2.1.1: "GHS09 Περιβαλλοντικός κίνδυνος")
}

// ── Lookup API ────────────────────────────────────────────────────────────────

// hStatementText resolves an H-statement code (e.g. "H317") to its official
// text in the requested language. Lookup is case- and trim-insensitive; an
// unrecognised Lang falls back to Greek. Returns ("", false) for unknown codes
// so the caller can FLAG for manual entry rather than emit a paraphrase.
func hStatementText(code string, lang Lang) (string, bool) {
	if p, ok := hLookup[normHCode(code)]; ok {
		return p.pick(lang), true
	}
	return "", false
}

// euhStatementText resolves an EUH supplementary-statement code (e.g. "EUH208")
// to its official text in the requested language. Same matching and fallback
// rules as hStatementText.
func euhStatementText(code string, lang Lang) (string, bool) {
	if p, ok := euhLookup[normHCode(code)]; ok {
		return p.pick(lang), true
	}
	return "", false
}

// euh208NamedText builds the EUH208 supplementary statement naming the specific
// skin-sensitising substance(s) that trigger it. CLP Annex II §2.8 requires
// EUH208 to NAME the sensitiser(s) present below their H317 classification
// limit but at/above their EUH208 band — "Contains <names>. May produce an
// allergic reaction." — not the generic "contains a component" wording, which
// is used here only as a fallback when no names are available. Names are
// de-duplicated (case-insensitive) and joined with ", " in stable order.
func euh208NamedText(names []string, lang Lang) string {
	clean := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		t := strings.TrimSpace(n)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		clean = append(clean, t)
	}
	if len(clean) == 0 {
		text, _ := euhStatementText("EUH208", lang)
		return text
	}
	joined := strings.Join(clean, ", ")
	if lang == LangEN {
		return "Contains " + joined + ". May produce an allergic reaction."
	}
	return "Περιέχει " + joined + ". Μπορεί να προκαλέσει αλλεργική αντίδραση."
}

// pStatementText resolves a P-statement code, including combined codes
// (e.g. "P305+P351+P338"), to its official text in the requested language. The
// spaced form ("P305 + P351 + P338") normalises to the same key. Same matching
// and fallback rules as hStatementText.
func pStatementText(code string, lang Lang) (string, bool) {
	if p, ok := pLookup[normKey(code)]; ok {
		return p.pick(lang), true
	}
	return "", false
}

// hazardClassDisplay resolves a CLP hazard-class name (e.g. "Skin Sens. 1B",
// "Aquatic Chronic 2") to its display text in the requested language. Lookup is
// case- and whitespace-insensitive. Returns ("", false) for unknown classes.
func hazardClassDisplay(class string, lang Lang) (string, bool) {
	if p, ok := classLookup[normKey(class)]; ok {
		return p.pick(lang), true
	}
	return "", false
}

// signalWordText resolves the GHS signal word ("Danger" / "Warning",
// case-insensitive) to its display text in the requested language. Returns
// ("", false) for any other input.
func signalWordText(word string, lang Lang) (string, bool) {
	if p, ok := signalLookup[normKey(word)]; ok {
		return p.pick(lang), true
	}
	return "", false
}

// pictogramAlt resolves a GHS pictogram code ("GHS01".."GHS09") to short alt
// text in the requested language. Lookup is case- and trim-insensitive. Returns
// ("", false) for unknown codes.
func pictogramAlt(code string, lang Lang) (string, bool) {
	if p, ok := pictoLookup[normKey(code)]; ok {
		return p.pick(lang), true
	}
	return "", false
}

// ── H-code → GHS pictogram correspondence (CLP Annex I) ───────────────────────
//
// The pictogram an H-statement carries on the label is fixed by CLP 1272/2008
// Annex I (Tables in §2/§3/§4). Codes that carry NO pictogram — the
// "Warning"-only / no-signal classes (e.g. H229 aerosol-only, H362 lactation,
// H412/H413 aquatic chronic 3/4, H420 ozone) and every EUH / P statement — are
// simply ABSENT from this map, so pictogramForHCode returns ("", false) for them
// and the directive renderer shows a bare code chip with no icon.
//
// Repr sub-variants (H360F/D/FD/Fd/Df, H361f/d/fd) all carry GHS08, so the
// case-insensitive normalisation collapsing them to one key is harmless.
var hCodePictograms = map[string]string{
	// GHS01 exploding bomb — explosives; self-reactive / organic peroxide type A & B.
	"H200": "GHS01", "H201": "GHS01", "H202": "GHS01", "H203": "GHS01",
	"H204": "GHS01", "H205": "GHS01", "H240": "GHS01", "H241": "GHS01",

	// GHS02 flame — flammable gases/aerosols/liquids/solids, pyrophoric,
	// self-heating, self-reactive C–F, organic peroxide C–F, emits flammable gas,
	// desensitised explosives (H206/H207/H208 carry the flame, not the bomb).
	"H206": "GHS02", "H207": "GHS02", "H208": "GHS02",
	"H220": "GHS02", "H221": "GHS02", "H222": "GHS02", "H223": "GHS02",
	"H224": "GHS02", "H225": "GHS02", "H226": "GHS02", "H228": "GHS02",
	"H230": "GHS02", "H231": "GHS02", "H232": "GHS02", "H242": "GHS02",
	"H250": "GHS02", "H251": "GHS02", "H252": "GHS02", "H260": "GHS02",
	"H261": "GHS02",

	// GHS03 flame over circle — oxidising gases/liquids/solids.
	"H270": "GHS03", "H271": "GHS03", "H272": "GHS03",

	// GHS04 gas cylinder — gases under pressure.
	"H280": "GHS04", "H281": "GHS04",

	// GHS05 corrosion — corrosive to metals, skin corrosion 1, serious eye damage 1.
	"H290": "GHS05", "H314": "GHS05", "H318": "GHS05",

	// GHS06 skull and crossbones — acute toxicity 1–3 (oral/dermal/inhalation),
	// including the multi-route combined statements (all components Cat 1–3).
	"H300": "GHS06", "H301": "GHS06", "H310": "GHS06", "H311": "GHS06",
	"H330": "GHS06", "H331": "GHS06",
	"H300+H310": "GHS06", "H300+H330": "GHS06", "H300+H310+H330": "GHS06",
	"H301+H311": "GHS06", "H301+H331": "GHS06", "H301+H311+H331": "GHS06",
	"H310+H330": "GHS06", "H311+H331": "GHS06",

	// GHS07 exclamation mark — acute tox 4, skin/eye irritation 2, STOT SE 3
	// (respiratory irritation / narcotic effects), skin sensitisation 1,
	// including the multi-route combined Cat-4 statements.
	"H302": "GHS07", "H312": "GHS07", "H315": "GHS07", "H317": "GHS07",
	"H319": "GHS07", "H332": "GHS07", "H335": "GHS07", "H336": "GHS07",
	"H302+H312": "GHS07", "H302+H332": "GHS07", "H302+H312+H332": "GHS07",
	"H312+H332": "GHS07",

	// GHS08 health hazard — aspiration, respiratory sensitisation, CMR,
	// STOT SE 1/2, STOT RE 1/2.
	"H304": "GHS08", "H334": "GHS08", "H340": "GHS08", "H341": "GHS08",
	"H350": "GHS08", "H350i": "GHS08", "H351": "GHS08", "H360": "GHS08",
	"H360F": "GHS08", "H360D": "GHS08", "H360FD": "GHS08", "H360Fd": "GHS08",
	"H360Df": "GHS08", "H361": "GHS08", "H361f": "GHS08", "H361d": "GHS08",
	"H361fd": "GHS08", "H370": "GHS08", "H371": "GHS08", "H372": "GHS08",
	"H373": "GHS08",

	// GHS09 environment — aquatic acute 1 (H400) and aquatic chronic 1 (H410)
	// ONLY. Aquatic Chronic 2 (H411), Chronic 3 (H412) and Chronic 4 (H413)
	// carry NO pictogram and NO signal word (CLP 1272/2008 Annex I Table 4.1.0
	// / Annex V), so H411 is deliberately ABSENT from this map.
	"H400": "GHS09", "H410": "GHS09",
}

// hCodePictoLk is the normalised (upper-cased, whitespace-stripped) view of
// hCodePictograms, built once in init so a query passed through normKey matches
// regardless of casing (e.g. "h350i" → "H350I").
var hCodePictoLk = map[string]string{}

func init() {
	for code, picto := range hCodePictograms {
		hCodePictoLk[normKey(code)] = picto
	}
}

// pictogramForHCode returns the GHS pictogram code (e.g. "GHS07") that an
// H-statement carries on the CLP label, and true; or ("", false) when the
// statement carries no pictogram (the "Warning"-only / no-signal classes) and
// for any non-H code (EUH / P statements never carry a pictogram). Lookup is
// case- and whitespace-insensitive. The correspondence is fixed by CLP
// 1272/2008 Annex I; this is the authority the directive renderer uses to decide
// whether a bare code chip gets an inline icon.
func pictogramForHCode(code string) (string, bool) {
	if p, ok := hCodePictoLk[normKey(code)]; ok {
		return p, true
	}
	return "", false
}
