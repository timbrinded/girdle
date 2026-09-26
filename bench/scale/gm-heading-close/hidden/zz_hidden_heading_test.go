package goldmark_test

import (
	"testing"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/testutil"
)

func TestZzHiddenATXClosingSequence(t *testing.T) {
	markdown := testutil.NewMarkdownToStringFunc(parser.New(), html.New())
	cases := map[string]string{
		"# Title #\n":        "<h1>Title</h1>\n",
		"## Title #\n":       "<h2>Title</h2>\n",
		"### Title ###\n":    "<h3>Title</h3>\n",
		"# Title #  \n":      "<h1>Title</h1>\n",
		"# Title#\n":         "<h1>Title#</h1>\n",
		"# Title \\#\n":      "<h1>Title #</h1>\n",
		"#### a #b #\n":      "<h4>a #b</h4>\n",
		"# #\n":              "<h1></h1>\n",
	}
	for src, want := range cases {
		got, err := markdown(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got != want {
			t.Errorf("%q rendered %q, want %q", src, got, want)
		}
	}
}
