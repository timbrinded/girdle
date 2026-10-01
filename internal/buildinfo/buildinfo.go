// Package buildinfo says which build of Girdle is running.
package buildinfo

import "runtime/debug"

// version is the release tag, set when a release is built:
//
//	-ldflags "-X github.com/timbrinded/girdle/internal/buildinfo.version=v0.1.0"
var version string

// Version is the release Girdle was built as. Other builds report the
// module version and commit Go recorded, such as a go install of a tag or a
// build from a checkout.
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	v := info.Main.Version
	for _, st := range info.Settings {
		switch {
		case st.Key == "vcs.revision":
			v += ", commit " + st.Value[:min(len(st.Value), 12)]
		case st.Key == "vcs.modified" && st.Value == "true":
			v += " with uncommitted changes"
		}
	}
	return v
}
