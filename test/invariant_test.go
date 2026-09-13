package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The descope seam only stays a one-line change if the invariant behind it
// holds: NO package outside internal/arbiter/anonymity.go may read the
// threshold value or perform the public/private classification itself.
//
// That invariant was written down in the design and then enforced by
// remembering. This test enforces it in CI instead — which is the difference
// between an invariant and an intention.
func TestInvariant_ThresholdIsReadOnlyInsideAnonymityFile(t *testing.T) {
	// Reading the boolean GateJustifications to decide whether an assertion
	// applies is fine — tests and the demo do it. Reading the NUMBER k, or
	// re-implementing the comparison, is not.
	forbidden := regexp.MustCompile(`(?i)(policy(\(\))?\.K\b|\bp\.K\b|Count\s*>=\s*[a-z]*\.?K\b)`)

	root := filepath.Join("..", "internal")
	allowed := filepath.Join("..", "internal", "arbiter", "anonymity.go")

	var violations []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if filepath.Clean(path) == filepath.Clean(allowed) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			code := line
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx] // comments may discuss k freely
			}
			if forbidden.MatchString(code) {
				violations = append(violations,
					filepath.ToSlash(path)+":"+itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("the anonymity threshold leaked outside internal/arbiter/anonymity.go.\n"+
			"Descoping anonymity must stay a single policy change; every site below "+
			"would need editing too:\n  %s", strings.Join(violations, "\n  "))
	}
}

// The other half of the boundary: the arbiter must not be able to reach a
// member's raw context. internal/agent/internal/rawctx is unreachable from it
// by Go's own visibility rules, so this test guards the interface side —
// arbiter.Store must expose no raw-context accessor.
func TestInvariant_ArbiterStoreExposesNoRawContext(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "internal", "arbiter", "ports.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "type Store interface {")
	if start < 0 {
		t.Fatal("arbiter.Store interface not found")
	}
	end := strings.Index(src[start:], "\n}")
	if end < 0 {
		t.Fatal("could not delimit the Store interface")
	}
	iface := src[start : start+end]

	for _, banned := range []string{"RawContext", "Raw(", "Profile(", "ProfileVector", "Derived("} {
		if strings.Contains(iface, banned) {
			t.Errorf("arbiter.Store exposes %q — the arbiter must have no path to a "+
				"member's private context", banned)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
