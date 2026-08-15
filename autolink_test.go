package mdflow_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
)

func TestCommonMarkAutolinkGrammar(t *testing.T) {
	p := mdflow.New()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "official example 606 rejects invalid email local part",
			src:  "<foo\\+@bar.example.com>\n",
			want: "<p>&lt;foo+@bar.example.com&gt;</p>\n",
		},
		{
			name: "official example 609 rejects one-character scheme",
			src:  "<m:abc>\n",
			want: "<p>&lt;m:abc&gt;</p>\n",
		},
		{
			name: "two-character scheme is valid",
			src:  "<ab:x>\n",
			want: "<p><a href=\"ab:x\">ab:x</a></p>\n",
		},
		{
			name: "32-character scheme is valid",
			src:  "<" + strings.Repeat("a", 32) + ":x>\n",
			want: "<p><a href=\"" + strings.Repeat("a", 32) + ":x\">" + strings.Repeat("a", 32) + ":x</a></p>\n",
		},
		{
			name: "33-character scheme is invalid",
			src:  "<" + strings.Repeat("a", 33) + ":x>\n",
			want: "<p>&lt;" + strings.Repeat("a", 33) + ":x&gt;</p>\n",
		},
		{
			name: "domain label cannot start with hyphen",
			src:  "<foo@-bar.example>\n",
			want: "<p>&lt;foo@-bar.example&gt;</p>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.HTML(tt.src); got != tt.want {
				t.Fatalf("HTML(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
