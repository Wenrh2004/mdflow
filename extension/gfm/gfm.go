// Package gfm bundles the GitHub-flavoured Markdown extensions — tables,
// strikethrough and task lists — into one composable [extension.Set].
package gfm

import (
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/strikethrough"
	"github.com/Wenrh2004/mdflow/extension/table"
	"github.com/Wenrh2004/mdflow/extension/tasklist"
)

// GFM is the GitHub-flavoured bundle: tables, strikethrough and task lists.
var GFM = extension.Set{table.Table, strikethrough.Strikethrough, tasklist.TaskList}
