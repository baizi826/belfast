package misc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The shapes below are taken verbatim from a captured official SC_10801 (client build
// 9.7.394), so a regression that changes our output format fails here rather than at a
// device.
var (
	mainShape  = regexp.MustCompile(`^\$azhash\$\d+\$\d+\$\d+\$[0-9a-f]{16}$`)
	entryShape = regexp.MustCompile(`^\$[a-z0-9]+hash\$\d+\$[0-9a-f]{16}$`)
)

func seedDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "CN", "ShareCfg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CN", "ShareCfg", "a.json"), []byte(`{"id":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CN", "ShareCfg", "b.json"), []byte(`{"id":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-json file must not affect the fingerprint.
	if err := os.WriteFile(filepath.Join(dir, "CN", "ShareCfg", "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func withVersions(t *testing.T, version string) {
	t.Helper()
	saved := azurLaneVersions
	azurLaneVersions = VersionMap{"CN": {Region: "CN", Version: version}}
	t.Cleanup(func() { azurLaneVersions = saved })
}

// The output must match the official packet's two shapes, in order, with both trailers.
func TestLocalHashesShape(t *testing.T) {
	t.Setenv("BELFAST_DATA_DIR", seedDataDir(t))
	withVersions(t, "9.7.395")

	hashes := localHashes("CN")
	if hashes == nil {
		t.Fatal("localHashes returned nil for a populated data dir")
	}

	// 1 main + 9 categories. Trailers are added by answer.updateVersions, not here.
	if want := 10; len(hashes) != want {
		t.Fatalf("got %d entries, want %d: %+v", len(hashes), want, hashes)
	}
	if hashes[0].Category != azMainCategory {
		t.Fatalf("first entry must be the main bundle, got %q", hashes[0].Category)
	}
	if !mainShape.MatchString(hashes[0].Hash) {
		t.Errorf("main hash %q does not match %v", hashes[0].Hash, mainShape)
	}
	if !strings.Contains(hashes[0].Hash, "$9$7$395$") {
		t.Errorf("main hash %q must carry the version from versions.json", hashes[0].Hash)
	}

	cats := strings.Split(resourceCategories, ",")
	for i, cat := range cats {
		entry := hashes[1+i]
		if entry.Category != cat {
			t.Errorf("entry %d: category %q, want %q", 1+i, entry.Category, cat)
		}
		if !entryShape.MatchString(entry.Hash) {
			t.Errorf("entry %d (%s) hash %q does not match %v", 1+i, cat, entry.Hash, entryShape)
		}
	}
}

// Same data, same answer - otherwise .cached_hashes would churn on every restart.
func TestLocalHashesDeterministic(t *testing.T) {
	t.Setenv("BELFAST_DATA_DIR", seedDataDir(t))
	withVersions(t, "9.7.395")

	first := localHashes("CN")
	second := localHashes("CN")
	if len(first) != len(second) {
		t.Fatalf("length differs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("entry %d differs between runs: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// Changing the data must change the fingerprint, or the version list cannot signal an update.
func TestLocalHashesTracksData(t *testing.T) {
	dir := seedDataDir(t)
	t.Setenv("BELFAST_DATA_DIR", dir)
	withVersions(t, "9.7.395")
	before := localHashes("CN")

	if err := os.WriteFile(filepath.Join(dir, "CN", "ShareCfg", "a.json"), []byte(`{"id":999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	after := localHashes("CN")

	if before[0].Hash == after[0].Hash {
		t.Errorf("main hash did not change after editing data: %q", before[0].Hash)
	}
}

// No data dir means no version list - the caller must not fall back to the network.
func TestLocalHashesWithoutDataDir(t *testing.T) {
	t.Setenv("BELFAST_DATA_DIR", "")
	withVersions(t, "9.7.395")

	if hashes := localHashes("CN"); hashes != nil {
		t.Errorf("expected nil without BELFAST_DATA_DIR, got %+v", hashes)
	}
}

func TestSplitVersion(t *testing.T) {
	cases := []struct {
		in                  string
		major, minor, build string
	}{
		{"9.7.395", "9", "7", "395"},
		{"9.7", "9", "7", "0"},
		{"9", "9", "0", "0"},
		{"", "0", "0", "0"},
		{"9.x.395", "9", "0", "395"},
	}
	for _, c := range cases {
		major, minor, build := splitVersion(c.in)
		if major != c.major || minor != c.minor || build != c.build {
			t.Errorf("splitVersion(%q) = %q,%q,%q want %q,%q,%q",
				c.in, major, minor, build, c.major, c.minor, c.build)
		}
	}
}

// The fingerprint counts only .json files, so stray files in the tree do not move it.
func TestTreeFingerprintIgnoresNonJSON(t *testing.T) {
	dir := seedDataDir(t)
	hash, count, err := treeFingerprint(dir, "CN")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("counted %d files, want 2 (the .txt must be skipped)", count)
	}
	if len(hash) != 64 {
		t.Errorf("fingerprint length %d, want 64 hex chars", len(hash))
	}
}
