// Package memos bundles the Memos-flavoured Markdown extensions — math,
// hashtags, typography wrappers and wiki-style resources — into one composable
// [extension.Set].
//
// The order matches github.com/usememos/gomark and the original mdflow default:
// math, then hashtags, then the typography wrappers, then resources.
package memos

import (
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/extension/hashtag"
	"github.com/Wenrh2004/mdflow/extension/math"
	"github.com/Wenrh2004/mdflow/extension/resource"
	"github.com/Wenrh2004/mdflow/extension/typography"
)

// Memos is the Memos-flavoured bundle: math, hashtags, typography and resources.
var Memos = extension.Set{
	math.Math,
	hashtag.Hashtag,
	typography.Typography,
	resource.Resource,
}
