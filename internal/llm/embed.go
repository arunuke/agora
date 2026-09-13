package llm

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// Dims is the embedding width. Small on purpose: the catalog is a few hundred
// titles and the vectors exist to map phrasing onto a closed vocabulary, not
// to do open-domain semantic search.
const Dims = 128

// DeterministicEmbedder produces a hashed bag-of-words vector.
//
// It is deliberately NOT a random projection: cosine similarity between two
// texts reflects real lexical overlap, so it is a usable offline proxy for
// paraphrase detection in the isolation gate and a usable nearest-neighbour
// signal for tier 2. It needs no network, so the whole suite stays hermetic.
type DeterministicEmbedder struct{}

func (DeterministicEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = embedOne(t)
	}
	return out, nil
}

func embedOne(text string) []float32 {
	v := make([]float32, Dims)
	for _, tok := range tokenize(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		v[h.Sum32()%Dims] += 1
		// A second, offset bucket reduces collision damage at this width.
		h2 := fnv.New32a()
		_, _ = h2.Write([]byte("#" + tok))
		v[h2.Sum32()%Dims] += 0.5
	}
	return normalize(v)
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var out []string
	for _, f := range fields {
		if len(f) < 2 || stopwords[f] {
			continue
		}
		out = append(out, stem(f))
	}
	return out
}

// stem is a crude suffix trimmer. It exists so that "comedies"/"comedy" and
// "documentaries"/"documentary" collide, which is what makes the variant-leak
// check in the isolation gate meaningful.
func stem(w string) string {
	for _, suf := range []string{"ies", "ing", "ed", "es", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			if suf == "ies" {
				return w[:len(w)-3] + "y"
			}
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true, "this": true,
	"you": true, "your": true, "are": true, "was": true, "have": true, "had": true,
	"but": true, "not": true, "can": true, "would": true, "like": true, "really": true,
	"want": true, "into": true, "some": true, "something": true, "any": true,
}

func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	n := float32(math.Sqrt(sum))
	for i := range v {
		v[i] /= n
	}
	return v
}

// Cosine assumes both vectors are L2-normalized, which Embed guarantees.
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var d float64
	for i := range a {
		d += float64(a[i]) * float64(b[i])
	}
	return d
}
