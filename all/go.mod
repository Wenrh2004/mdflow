module github.com/Wenrh2004/mdflow/all

go 1.26.0


require (
	github.com/Wenrh2004/mdflow v0.0.0
	github.com/Wenrh2004/mdflow/extension/gfm v0.0.0
	github.com/Wenrh2004/mdflow/extension/memos v0.0.0
	github.com/Wenrh2004/mdflow/extension/rawhtml v0.0.0
)

require (
	github.com/Wenrh2004/mdflow/extension/hashtag v0.0.0 // indirect
	github.com/Wenrh2004/mdflow/extension/math v0.0.0 // indirect
	github.com/Wenrh2004/mdflow/extension/resource v0.0.0 // indirect
	github.com/Wenrh2004/mdflow/extension/strikethrough v0.0.0 // indirect
	github.com/Wenrh2004/mdflow/extension/table v0.0.0 // indirect
	github.com/Wenrh2004/mdflow/extension/typography v0.0.0 // indirect
)

replace github.com/Wenrh2004/mdflow => ../

replace github.com/Wenrh2004/mdflow/extension/gfm => ../extension/gfm

replace github.com/Wenrh2004/mdflow/extension/memos => ../extension/memos

replace github.com/Wenrh2004/mdflow/extension/rawhtml => ../extension/rawhtml

replace github.com/Wenrh2004/mdflow/extension/table => ../extension/table

replace github.com/Wenrh2004/mdflow/extension/strikethrough => ../extension/strikethrough

replace github.com/Wenrh2004/mdflow/extension/math => ../extension/math

replace github.com/Wenrh2004/mdflow/extension/hashtag => ../extension/hashtag

replace github.com/Wenrh2004/mdflow/extension/typography => ../extension/typography

replace github.com/Wenrh2004/mdflow/extension/resource => ../extension/resource
