// Package vocab is the closed constraint vocabulary.
//
// It is the single source of truth for both loops and for the k-threshold.
// The vocabulary is CLOSED on purpose: the anonymity threshold requires
// countable, EQUAL constraints. You cannot reliably determine that "wants
// something short" and "prefers brief films" are the same constraint if they
// are free text. Anonymity requires countability; countability requires a
// closed set.
package vocab

import (
	"fmt"
	"sort"
	"strings"
)

type Dim string

const (
	DimGenre        Dim = "genre"
	DimEra          Dim = "era"
	DimRuntimeMax   Dim = "runtime_max_min"
	DimTone         Dim = "tone"
	DimLanguage     Dim = "language"
	DimAvailability Dim = "availability"
	DimMaturity     Dim = "maturity"
)

type Polarity string

const (
	Prefer  Polarity = "prefer"
	Exclude Polarity = "exclude"
)

// Constraint is a single derived preference. It carries no member identity and
// no free text — only closed-vocabulary values.
type Constraint struct {
	Dim      Dim      `json:"dim"`
	Value    string   `json:"value"`
	Polarity Polarity `json:"polarity"`
	Weight   float64  `json:"weight"`
}

// Key identifies a constraint for tallying. Weight is deliberately excluded:
// two members who both want comedy hold the SAME constraint even if one wants
// it more, and the k-threshold must count them as two.
func (c Constraint) Key() string {
	return fmt.Sprintf("%s:%s:%s", c.Dim, c.Value, c.Polarity)
}

func (c Constraint) Human() string {
	t := Lookup(c.Dim, c.Value)
	if t == nil {
		return string(c.Dim) + " " + c.Value
	}
	if c.Polarity == Exclude {
		return "not " + t.Phrase
	}
	return t.Phrase
}

// Veto is a hard exclusion. It is a separate list rather than a polarity
// because it is a filter, not a weight — it cannot be outvoted.
type Veto struct {
	Dim   Dim    `json:"dim"`
	Value string `json:"value"`
}

func (v Veto) Key() string { return fmt.Sprintf("%s:%s", v.Dim, v.Value) }

// Term is a canonical vocabulary entry. Synonyms drive tier-1 and tier-3
// extraction; Phrase drives human-readable justifications; the term text is
// embedded once at seed time to drive tier-2 extraction.
type Term struct {
	Dim      Dim
	Value    string
	Phrase   string
	Synonyms []string
}

func (t Term) EmbedText() string {
	return t.Phrase + " " + strings.Join(t.Synonyms, " ")
}

var terms = []Term{
	{DimGenre, "comedy", "comedies", []string{"comedy", "comedies", "funny", "laugh", "humor", "humour", "sitcom"}},
	{DimGenre, "horror", "horror", []string{"horror", "scary", "frightening", "jump scares", "slasher", "creepy"}},
	{DimGenre, "drama", "drama", []string{"drama", "dramatic", "character study", "serious film"}},
	{DimGenre, "scifi", "science fiction", []string{"scifi", "sci-fi", "science fiction", "space", "futuristic", "dystopian"}},
	{DimGenre, "animation", "animation", []string{"animation", "animated", "cartoon", "anime", "claymation"}},
	{DimGenre, "documentary", "documentaries", []string{"documentary", "documentaries", "docs", "nonfiction", "non-fiction"}},
	{DimGenre, "thriller", "thrillers", []string{"thriller", "suspense", "suspenseful", "tense"}},
	{DimGenre, "romance", "romance", []string{"romance", "romantic", "rom-com", "love story"}},

	{DimEra, "pre_1970", "older classics", []string{"classic", "classics", "old movies", "black and white", "golden age"}},
	{DimEra, "1970s", "seventies films", []string{"1970s", "70s", "seventies"}},
	{DimEra, "1980s", "eighties films", []string{"1980s", "80s", "eighties"}},
	{DimEra, "1990s", "nineties films", []string{"1990s", "90s", "nineties"}},
	{DimEra, "2000s", "2000s films", []string{"2000s", "noughties", "early 2000s"}},
	{DimEra, "2010s", "2010s films", []string{"2010s"}},
	{DimEra, "2020s", "recent releases", []string{"2020s", "recent", "new releases", "modern", "latest"}},

	{DimRuntimeMax, "90", "something short", []string{"short", "quick", "under 90", "ninety minutes", "brief", "under an hour and a half"}},
	{DimRuntimeMax, "120", "under two hours", []string{"under two hours", "two hours", "not too long", "under 120"}},
	{DimRuntimeMax, "150", "up to two and a half hours", []string{"two and a half hours", "under 150"}},
	{DimRuntimeMax, "999", "long films", []string{"long", "epic", "lengthy", "marathon"}},

	{DimTone, "light", "something light", []string{"light", "lighthearted", "light-hearted", "feel good", "feel-good", "easy watch", "uplifting"}},
	{DimTone, "dark", "darker material", []string{"dark", "bleak", "grim", "heavy", "downbeat"}},
	{DimTone, "cozy", "cozy films", []string{"cozy", "cosy", "comforting", "warm", "autumnal", "comfort watch"}},
	{DimTone, "intense", "intense films", []string{"intense", "gripping", "edge of my seat", "adrenaline"}},
	{DimTone, "weird", "offbeat films", []string{"weird", "strange", "surreal", "offbeat", "quirky", "bizarre"}},

	{DimLanguage, "en", "English-language films", []string{"english"}},
	{DimLanguage, "ko", "Korean films", []string{"korean", "korea", "k-drama"}},
	{DimLanguage, "ja", "Japanese films", []string{"japanese", "japan"}},
	{DimLanguage, "es", "Spanish-language films", []string{"spanish", "spain", "latin american"}},
	{DimLanguage, "fr", "French films", []string{"french", "france"}},
	{DimLanguage, "other", "films in other languages", []string{"foreign", "subtitles", "subtitled"}},

	{DimAvailability, "subscription", "things included with a subscription", []string{"subscription", "included", "streaming", "no extra cost"}},
	{DimAvailability, "rental", "rentals", []string{"rental", "rent", "paid"}},
	{DimAvailability, "free_with_ads", "free with ads", []string{"free", "with ads", "ad supported", "ad-supported"}},

	{DimMaturity, "all_ages", "something for all ages", []string{"all ages", "family friendly", "family-friendly", "kid friendly", "kids", "children"}},
	{DimMaturity, "teen", "teen-appropriate films", []string{"teen", "teenager", "pg-13"}},
	{DimMaturity, "adult", "adult films", []string{"adult", "grown up", "grown-up", "mature"}},
}

func Terms() []Term { return terms }

func Lookup(d Dim, value string) *Term {
	for i := range terms {
		if terms[i].Dim == d && terms[i].Value == value {
			return &terms[i]
		}
	}
	return nil
}

func Valid(d Dim, value string) bool { return Lookup(d, value) != nil }

// Negation markers. "Strong" markers produce a veto; "soft" markers produce an
// exclude. The distinction matters: a veto is a hard filter that is silently
// applied, an exclude is a weight that can be outranked.
var (
	strongNegation = []string{"can't", "cannot", "cant", "never", "hate", "refuse", "absolutely not", "no way"}
	softNegation   = []string{"not ", "no ", "don't", "dont", "avoid", "rather not", "dislike", "prefer not", "skip", "less "}
	intensifiers   = []string{"really", "love", "adore", "always", "definitely", "very", "huge fan"}
)

func HasStrongNegation(s string) bool { return containsAny(s, strongNegation) }
func HasSoftNegation(s string) bool   { return containsAny(s, softNegation) }
func HasIntensifier(s string) bool    { return containsAny(s, intensifiers) }

func containsAny(s string, subs []string) bool {
	s = strings.ToLower(s)
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// MatchTerms returns every vocabulary term whose synonyms appear in the text.
func MatchTerms(text string) []Term {
	t := strings.ToLower(text)
	var out []Term
	seen := map[string]bool{}
	for _, term := range terms {
		for _, syn := range term.Synonyms {
			if strings.Contains(t, syn) {
				k := string(term.Dim) + ":" + term.Value
				if !seen[k] {
					seen[k] = true
					out = append(out, term)
				}
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dim != out[j].Dim {
			return out[i].Dim < out[j].Dim
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// SplitClauses breaks an utterance so that negation scopes correctly.
// "I like comedies but I can't do horror" must not mark comedy as vetoed.
func SplitClauses(text string) []string {
	repl := strings.NewReplacer(
		" but ", "|", " and ", "|", ", ", "|", ". ", "|", "; ", "|", " though ", "|", " however ", "|",
	)
	parts := strings.Split(repl.Replace(strings.ToLower(text)), "|")
	var out []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return []string{strings.ToLower(text)}
	}
	return out
}
