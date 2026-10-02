package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

// markdown renders the agent's replies. A renderer is built for a width and
// background and reused until either changes.
type markdown struct {
	width int
	dark  bool
	r     *glamour.TermRenderer
}

func (md *markdown) render(text string, width int) string {
	if md.r == nil || md.width != width || md.dark != isDark {
		cfg := styleConfig(isDark)
		r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(width))
		if err != nil {
			return text
		}
		md.r, md.width, md.dark = r, width, isDark
	}
	out, err := md.r.Render(text)
	if err != nil {
		return text
	}
	// Glamour pads blocks with blank lines, which would separate a reply
	// from its label.
	lines := strings.Split(out, "\n")
	blank := func(l string) bool { return strings.TrimSpace(xansi.Strip(l)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// styleConfig is Glamour's standard style with the document's margins
// removed, so replies line up with the rest of the transcript, and the
// accents taken from the palette.
func styleConfig(dark bool) ansi.StyleConfig {
	cfg := styles.LightStyleConfig
	if dark {
		cfg = styles.DarkStyleConfig
	}
	zero := uint(0)
	hex := func(c color.Color) *string {
		r, g, b, _ := c.RGBA()
		s := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
		return &s
	}
	cfg.Document.Margin = &zero
	cfg.Document.BlockPrefix, cfg.Document.BlockSuffix = "", ""
	cfg.Document.Color = hex(pal.fg)
	cfg.Heading.Color = hex(pal.brand)
	cfg.H1.Color, cfg.H1.BackgroundColor = hex(pal.fg), nil
	cfg.H1.Prefix, cfg.H1.Suffix = "", ""
	for _, h := range []*ansi.StyleBlock{&cfg.H2, &cfg.H3, &cfg.H4, &cfg.H5, &cfg.H6} {
		h.Prefix = ""
	}
	cfg.Code.Color = hex(pal.belt)
	cfg.Code.BackgroundColor = nil
	cfg.Code.Prefix, cfg.Code.Suffix = "", ""
	cfg.Link.Color = hex(pal.user)
	cfg.LinkText.Color = hex(pal.user)
	cfg.Item.BlockPrefix = "• "
	return cfg
}
