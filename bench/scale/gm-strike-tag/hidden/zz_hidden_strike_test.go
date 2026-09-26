package extension

import (
	"testing"

	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/testutil"
)

func zzHiddenRender(t *testing.T, r html.Extension, src string) string {
	t.Helper()
	markdown := testutil.NewMarkdownToStringFunc(
		parser.New(parser.WithExtensions(NewStrikethroughParser())),
		html.New(html.WithExtensions(r)),
	)
	out, err := markdown(src)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestZzHiddenStrikethroughTag(t *testing.T) {
	if got, want := zzHiddenRender(t, NewStrikethroughHTMLRenderer(WithStrikethroughHTMLTag("s")), "~~gone~~ and ~~this~~\n"),
		"<p><s>gone</s> and <s>this</s></p>\n"; got != want {
		t.Errorf("with tag s: got %q, want %q", got, want)
	}
	if got, want := zzHiddenRender(t, NewStrikethroughHTMLRenderer(), "~~gone~~\n"), "<p><del>gone</del></p>\n"; got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
	if got, want := zzHiddenRender(t, StrikethroughHTMLRenderer, "~~gone~~\n"), "<p><del>gone</del></p>\n"; got != want {
		t.Errorf("package default: got %q, want %q", got, want)
	}
}
