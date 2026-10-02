package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.1", "v0.1.0", true},
		{"v0.10.0", "v0.9.0", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		// Builds that aren't releases are never behind.
		{"v0.2.0", "(devel)", false},
		{"v0.2.0", "v0.1.1-0.20261002090000-abcdef123456+dirty, commit abcdef123456", false},
		{"v0.2.0", "v0.2.0-rc.1", false},
		{"", "v0.1.0", false},
	} {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

// serveRelease serves a release shaped as GoReleaser publishes it, holding a
// binary with the given contents, and counts requests for the latest
// release.
func serveRelease(t *testing.T, tag string, binary []byte, tamper bool) (Source, *atomic.Int32) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range map[string][]byte{"girdle": binary, "LICENSE": []byte("Apache-2.0")} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(data)
	}
	tw.Close()
	gz.Close()
	archive := fmt.Sprintf("girdle_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(buf.Bytes())
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archive)
	if tamper {
		buf.WriteByte(0)
	}

	var asked atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	})
	mux.HandleFunc("/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(sums)) })
	mux.HandleFunc("/download/"+tag+"/"+archive, func(w http.ResponseWriter, _ *http.Request) { w.Write(buf.Bytes()) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return Source{Latest: srv.URL + "/latest", Download: srv.URL + "/download", HTTP: srv.Client()}, &asked
}

func TestInstall(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "tampered"}[tamper], func(t *testing.T) {
			src, _ := serveRelease(t, "v0.2.0", []byte("new binary"), tamper)
			path := filepath.Join(t.TempDir(), "girdle")
			if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
				t.Fatal(err)
			}

			err := src.Install(t.Context(), "v0.2.0", path)
			got, _ := os.ReadFile(path)
			switch {
			case tamper && (err == nil || string(got) != "old binary"):
				t.Fatalf("tampered archive: err = %v, binary = %q", err, got)
			case !tamper && (err != nil || string(got) != "new binary"):
				t.Fatalf("err = %v, binary = %q", err, got)
			}
			if info, _ := os.Stat(path); info.Mode().Perm()&0o100 == 0 {
				t.Fatal("binary isn't executable")
			}
			if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".girdle-update-*")); len(left) > 0 {
				t.Fatalf("left behind %v", left)
			}
		})
	}
}

func TestNotice(t *testing.T) {
	t.Setenv("GIRDLE_NO_UPDATE_CHECK", "")
	src, asked := serveRelease(t, "v0.2.0", nil, false)
	cache := filepath.Join(t.TempDir(), "update.json")

	for range 2 {
		if got := src.Notice(t.Context(), "v0.1.0", cache); !strings.Contains(got, "v0.2.0") || !strings.Contains(got, "girdle update") {
			t.Fatalf("Notice = %q", got)
		}
	}
	if n := asked.Load(); n != 1 {
		t.Fatalf("asked for the latest release %d times in a day, want 1", n)
	}
	if got := src.Notice(t.Context(), "v0.2.0", cache); got != "" {
		t.Fatalf("up to date, but Notice = %q", got)
	}
	if got := src.Notice(t.Context(), "(devel)", filepath.Join(t.TempDir(), "u.json")); got != "" || asked.Load() != 1 {
		t.Fatalf("a development build got %q and asked the source", got)
	}
}
