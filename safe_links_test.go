package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

// WithSafeLinks must neutralise dangerous URI schemes in link and image
// destinations — including the entity-obfuscated forms the parser decodes
// before the destination reaches the renderer, which a source-level filter
// would miss. The dangerous scheme must never appear inside an href/src.
func TestSafeLinksNeutralisesDangerousSchemes(t *testing.T) {
	safe := mdflow.New(mdflow.WithSafeLinks())

	cases := []string{
		"[x](javascript:alert(1))",
		"[x](java&#115;cript:alert(1))",   // entity-obfuscated 's'
		"[x](&#106;avascript:alert(1))",   // entity-obfuscated 'j'
		"[x](javascript&#58;alert(1))",    // entity-obfuscated ':'
		"[x](JaVaScRiPt:alert(1))",        // case trick
		"![x](javascript:alert(1))",       // image
		"<javascript:alert(1)>",           // autolink
		"[x](vbscript:msgbox(1))",
		"[x](data:text/html,<b>hi</b>)",
	}
	dangerous := []string{`="javascript:`, `="vbscript:`, `="data:`}
	for _, md := range cases {
		out := safe.HTML(md)
		for _, bad := range dangerous {
			if strings.Contains(out, bad) {
				t.Errorf("safe-links leaked %q in output for %q:\n%s", bad, md, out)
			}
		}
	}
}

// The allowlist must let ordinary destinations through unchanged.
func TestSafeLinksPreservesSafeDestinations(t *testing.T) {
	safe := mdflow.New(mdflow.WithSafeLinks())
	for _, tc := range []struct{ md, want string }{
		{"[x](https://example.com)", `href="https://example.com"`},
		{"[x](http://example.com)", `href="http://example.com"`},
		{"[x](/relative/path)", `href="/relative/path"`},
		{"[x](#fragment)", `href="#fragment"`},
		{"[x](mailto:a@b.com)", `href="mailto:a@b.com"`},
		{"[x](tel:+15551234)", `href="tel:+15551234"`},
	} {
		if out := safe.HTML(tc.md); !strings.Contains(out, tc.want) {
			t.Errorf("safe-links dropped a safe destination for %q: want %q in\n%s", tc.md, tc.want, out)
		}
	}
}

// Without the option the default profile stays byte-for-byte CommonMark: the
// dangerous scheme (including the decoded-entity form) passes through. This
// pins the opt-in boundary and documents the default behaviour.
func TestDefaultProfileDoesNotFilterSchemes(t *testing.T) {
	def := mdflow.New()
	if out := def.HTML("[x](javascript:alert(1))"); !strings.Contains(out, `href="javascript:alert(1)"`) {
		t.Errorf("default profile unexpectedly altered a scheme:\n%s", out)
	}
	if out := def.HTML("[x](java&#115;cript:alert(1))"); !strings.Contains(out, `href="javascript:alert(1)"`) {
		t.Errorf("default profile should decode the entity to the raw scheme:\n%s", out)
	}
}
