// Package update finds newer Girdle releases and installs them over the
// running binary.
package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Source is where releases come from.
type Source struct {
	// Latest is the API URL describing the latest release.
	Latest string
	// Download is the URL release files sit under, as <Download>/<tag>/<file>.
	Download string
	HTTP     *http.Client
}

// GitHub is where Girdle's releases are published.
var GitHub = Source{
	Latest:   "https://api.github.com/repos/timbrinded/girdle/releases/latest",
	Download: "https://github.com/timbrinded/girdle/releases/download",
	HTTP:     http.DefaultClient,
}

// maxDownload bounds a release file, well above the size of an archive.
const maxDownload = 256 << 20

// LatestTag returns the latest release's tag.
func (s Source) LatestTag(ctx context.Context) (string, error) {
	body, err := s.get(ctx, s.Latest)
	if err != nil {
		return "", err
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil || release.Tag == "" {
		return "", fmt.Errorf("unreadable release from %s", s.Latest)
	}
	return release.Tag, nil
}

// Install downloads release tag for this platform, checks it against the
// release's checksums, and replaces the binary at path with it.
func (s Source) Install(ctx context.Context, tag, path string) error {
	archive := fmt.Sprintf("girdle_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	base := s.Download + "/" + tag + "/"
	sums, err := s.get(ctx, base+"checksums.txt")
	if err != nil {
		return err
	}
	data, err := s.get(ctx, base+archive)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if want := checksum(sums, archive); want == "" || want != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("%s doesn't match the release's checksum", archive)
	}
	bin, err := extract(data, "girdle")
	if err != nil {
		return fmt.Errorf("%s: %w", archive, err)
	}
	// Write beside the binary, then rename over it: a rename within a
	// directory is atomic, so the binary is never half written.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".girdle-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(bin)
	err = cmp.Or(err, tmp.Chmod(0o755), tmp.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (s Source) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxDownload))
}

// checksum finds name's SHA-256 in a checksums file of "<hash>  <name>"
// lines.
func checksum(sums []byte, name string) string {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		if hash, file, ok := strings.Cut(sc.Text(), "  "); ok && file == name {
			return hash
		}
	}
	return ""
}

// extract returns the file called name from a .tar.gz archive.
func extract(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("no %s in the archive", name)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == name {
			return io.ReadAll(io.LimitReader(tr, maxDownload))
		}
	}
}

// Newer reports whether release latest is newer than version current. A
// current version that isn't a release, such as a build from a checkout, is
// never behind.
func Newer(latest, current string) bool {
	l, ok := release(latest)
	c, ok2 := release(current)
	return ok && ok2 && slices.Compare(l, c) > 0
}

// IsRelease reports whether version names a release, such as v0.1.0.
func IsRelease(version string) bool {
	_, ok := release(version)
	return ok
}

// release parses a vMAJOR.MINOR.PATCH tag. Pre-releases and Go's
// pseudo-versions for unreleased commits are not releases.
func release(v string) ([]int, bool) {
	v, ok := strings.CutPrefix(v, "v")
	parts := strings.Split(v, ".")
	if !ok || len(parts) != 3 {
		return nil, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// checkEvery is how often Notice asks for the latest release.
const checkEvery = 24 * time.Hour

// cache records the latest release Notice saw, and when it asked.
type cache struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest"`
}

// Notice says, in a sentence, that a newer release than current is out, or
// returns "". It asks the source at most once a day, keeping the answer in
// cachePath, and never for a build that isn't a release.
// GIRDLE_NO_UPDATE_CHECK turns it off.
func (s Source) Notice(ctx context.Context, current, cachePath string) string {
	if os.Getenv("GIRDLE_NO_UPDATE_CHECK") != "" || !IsRelease(current) {
		return ""
	}
	var c cache
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	if time.Since(c.Checked) >= checkEvery {
		// A failed check waits a day too, rather than slowing every start
		// while offline.
		c.Checked = time.Now()
		if tag, err := s.LatestTag(ctx); err == nil {
			c.Latest = tag
		}
		// A cache that can't be written only means asking again next time.
		if data, err := json.Marshal(c); err == nil && os.MkdirAll(filepath.Dir(cachePath), 0o755) == nil {
			_ = os.WriteFile(cachePath, data, 0o644)
		}
	}
	if !Newer(c.Latest, current) {
		return ""
	}
	return fmt.Sprintf("Girdle %s is available (you have %s). Run `girdle update` to update.", c.Latest, current)
}

// CachePath is where Notice keeps the latest release it saw.
func CachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(cmp.Or(os.Getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache")), "girdle", "update.json")
}
