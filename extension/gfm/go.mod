module github.com/Wenrh2004/mdflow/extension/gfm

go 1.26.0


require (
	github.com/Wenrh2004/mdflow v0.0.0
	github.com/Wenrh2004/mdflow/extension/strikethrough v0.0.0
	github.com/Wenrh2004/mdflow/extension/table v0.0.0
)

replace github.com/Wenrh2004/mdflow => ../../

replace github.com/Wenrh2004/mdflow/extension/table => ../table

replace github.com/Wenrh2004/mdflow/extension/strikethrough => ../strikethrough
