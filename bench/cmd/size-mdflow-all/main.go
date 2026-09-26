// Command size-mdflow-all links every bundled mdflow extension for
// TestBinarySize.
package main

import (
	"fmt"

	"github.com/Wenrh2004/mdflow/all"
)

func main() { fmt.Print(all.New().HTML("x")) }
