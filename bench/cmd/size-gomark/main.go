// Command size-gomark links gomark for TestBinarySize.
package main

import (
	"fmt"

	"github.com/usememos/gomark"
	"github.com/usememos/gomark/renderer/html"
)

func main() {
	doc, _ := gomark.Parse("x")
	fmt.Print(html.NewHTMLRenderer().RenderDocument(doc))
}
