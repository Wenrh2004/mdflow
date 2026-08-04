module github.com/Wenrh2004/mdflow/extension/memos

go 1.26.0


require (
	github.com/Wenrh2004/mdflow v0.0.0
	github.com/Wenrh2004/mdflow/extension/hashtag v0.0.0
	github.com/Wenrh2004/mdflow/extension/math v0.0.0
	github.com/Wenrh2004/mdflow/extension/resource v0.0.0
	github.com/Wenrh2004/mdflow/extension/typography v0.0.0
)

replace github.com/Wenrh2004/mdflow => ../../

replace github.com/Wenrh2004/mdflow/extension/math => ../math

replace github.com/Wenrh2004/mdflow/extension/hashtag => ../hashtag

replace github.com/Wenrh2004/mdflow/extension/typography => ../typography

replace github.com/Wenrh2004/mdflow/extension/resource => ../resource
