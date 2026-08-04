package mdflow

// Heading is one entry of a document outline, as returned by [Parser.Headings].
//
// It is a derived view rather than a parse node, which is why it lives here and
// not in package token — where the name is taken by the token.Heading node kind.
type Heading struct {
	Level int
	Text  string
}
