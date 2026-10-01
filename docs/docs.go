// Package docs embeds Girdle's user guides, so the girdle tool describes the
// build that is running rather than whatever version the model remembers.
package docs

import "embed"

// Guides holds the guides the girdle tool serves, by file name.
//
//go:embed getting-started.md usage.md configuration.md architecture.md
var Guides embed.FS
