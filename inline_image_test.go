package mdflow_test

import (
	"testing"

	"github.com/Wenrh2004/mdflow"
)

func TestNestedImageIdentityPipelineMatchesFastPath(t *testing.T) {
	const src = "![outer ![inner *strong*](/inner) [link](/link)](/outer \"title\")\n"
	base := mdflow.New()
	identity := base.Map(func(e mdflow.Event) mdflow.Event { return e })
	if got, want := identity.HTML(src), base.HTML(src); got != want {
		t.Fatalf("identity pipeline HTML(%q)\n got: %q\nwant: %q", src, got, want)
	}
}
