// Package gfm bundles the GitHub-flavoured Markdown extensions — tables and
// strikethrough — into one composable [extension.Set].
//
// Task lists are not here: their marker is a few lines inside the core list
// rule, so the CommonMark core already parses them.
package gfm

import (
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/strikethrough"
	"github.com/Wenrh2004/mdflow/extension/table"
)

// GFM is the GitHub-flavoured bundle: tables and strikethrough.
var GFM = extension.Set{table.Table, strikethrough.Strikethrough}
