// Package bench holds the comparative benchmarks against other Go markdown
// libraries. It lives in its own module so that the mdflow module itself stays
// dependency-free for the people who import it.
package bench

import (
	"strconv"
	"strings"
)

// Doc builds a document of n repeated sections. Every construct used here is
// supported by *both* parsers under test, so the comparison measures parsing
// speed rather than one library silently skipping syntax it does not implement.
//
// The mix approximates real technical writing: prose with inline markup carries
// most of the bytes, punctuated by headings, code, lists, quotes and a table.
func Doc(n int) string {
	var b strings.Builder
	b.Grow(n * 420)
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		b.WriteString("## Section " + s + "\n\n")
		b.WriteString("This paragraph has *emphasis*, **strong text**, `inline code` " +
			"and [a link](https://example.com/page/" + s + ") in it, plus enough ordinary " +
			"prose after the markup that the byte mix resembles real writing rather than " +
			"a stress test made only of delimiters.\n\n")
		b.WriteString("```go\nfunc handler" + s + "(w http.ResponseWriter, r *http.Request) {\n" +
			"\tw.WriteHeader(" + strconv.Itoa(200+i%100) + ")\n}\n```\n\n")
		b.WriteString("- first item\n- second item with `code`\n- third item\n\n")
		b.WriteString("- [x] shipped item " + s + "\n- [ ] pending item\n\n")
		b.WriteString("> A blockquote with **bold** text explaining section " + s + ".\n\n")
		b.WriteString("Tagged #section" + s + " with ==highlighted== terms, ~~struck~~ text " +
			"and inline math $a_" + s + " + b$ mixed into the prose.\n\n")
		b.WriteString("| name | value |\n| --- | ---: |\n| alpha | " + s + " |\n| beta | " + s + " |\n\n")
	}
	return b.String()
}

// ProseDoc is markup-light text: the case the trigger-byte fast path is built
// for, and the majority of bytes in most real documents.
func ProseDoc(n int) string {
	var b strings.Builder
	b.Grow(n * 300)
	for i := 0; i < n; i++ {
		b.WriteString("## Chapter " + strconv.Itoa(i) + "\n\n")
		b.WriteString("Plain prose with no inline markup whatsoever, the sort of " +
			"paragraph that makes up the bulk of documentation and blog posts. " +
			"It is here to measure how the parsers handle the common case rather " +
			"than the pathological one.\n\n")
	}
	return b.String()
}

// Sizes used by the size-scaling benchmarks: roughly 2 KB, 20 KB and 200 KB.
var Sizes = []struct {
	Name     string
	Sections int
}{
	{"small", 5},
	{"medium", 50},
	{"large", 500},
}
