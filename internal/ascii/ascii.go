// Package ascii holds the byte classifiers the parser, the raw-HTML
// capability and the HTML renderer all need. CommonMark defines its syntax in
// ASCII terms, so these deliberately ignore Unicode; one copy keeps the three
// packages from drifting apart on what counts as a letter.
package ascii

// IsAlpha reports whether c is an ASCII letter.
func IsAlpha(c byte) bool { return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' }

// IsDigit reports whether c is an ASCII digit.
func IsDigit(c byte) bool { return '0' <= c && c <= '9' }

// IsAlnum reports whether c is an ASCII letter or digit.
func IsAlnum(c byte) bool { return IsAlpha(c) || IsDigit(c) }

// Lower maps an ASCII upper-case letter to lower case and leaves every other
// byte alone.
func Lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
