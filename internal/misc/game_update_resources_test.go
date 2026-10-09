package misc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedResourceFile writes a client_resources.json into dir and returns dir.
func seedResourceFile(t *testing.T, dir string, entries []string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"version":"9.7.395","entries":[`)
	for i, e := range entries {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"` + e + `"`)
	}
	b.WriteString(`]}`)
	if err := os.WriteFile(filepath.Join(dir, clientResourcesFile), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The recorded list must win over the derived one, and must be passed through verbatim:
// these values identify index files on the official CDN, so any mangling breaks the
// client with "Hash文件校验失败".
func TestLocalHashesPrefersRecordedList(t *testing.T) {
	dir := seedDataDir(t)
	// Deliberately NOT the values the data tree would produce.
	recorded := []string{
		"$azhash$9$7$395$2a544b966565ede4",
		"$cvhash$1480$dac1efd2859ad69c",
		"$paintinghash$1800$2010b7cb86d14d76",
		"$maphash$333$059dc192332c5f56",
	}
	seedResourceFile(t, dir, recorded)
	t.Setenv("BELFAST_DATA_DIR", dir)
	withVersions(t, "9.7.395")

	got := localHashes("CN")
	if len(got) != len(recorded) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(recorded), got)
	}
	for i, want := range recorded {
		if got[i].Hash != want {
			t.Errorf("entry %d = %q, want verbatim %q", i, got[i].Hash, want)
		}
	}
	// Category must be recovered from the entry itself, since the file stores only the
	// whole `$<cat>hash$...` string.
	if got[0].Category != "az" {
		t.Errorf("main category = %q, want az", got[0].Category)
	}
	if got[1].Category != "cv" {
		t.Errorf("second category = %q, want cv", got[1].Category)
	}
	if got[3].Category != "map" {
		t.Errorf("last category = %q, want map", got[3].Category)
	}
}

// A missing file must fall back to the derived list, not return nil: an old data dir
// should still produce a well-formed packet.
func TestLocalHashesFallsBackWithoutRecordedList(t *testing.T) {
	t.Setenv("BELFAST_DATA_DIR", seedDataDir(t))
	withVersions(t, "9.7.395")

	got := localHashes("CN")
	if len(got) != 10 {
		t.Fatalf("fallback produced %d entries, want 10", len(got))
	}
	if !mainShape.MatchString(got[0].Hash) {
		t.Errorf("fallback main hash %q does not match the official shape", got[0].Hash)
	}
}

// Malformed input must not poison the packet: entries without the `$<cat>hash$` shape
// are dropped rather than emitted as-is.
func TestResourceEntriesRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	seedResourceFile(t, dir, []string{
		"$cvhash$1480$dac1efd2859ad69c",
		"garbage",
		"",
		"$maphash$333$059dc192332c5f56",
	})

	got := resourceEntries(dir)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if got[0] != "$cvhash$1480$dac1efd2859ad69c" || got[1] != "$maphash$333$059dc192332c5f56" {
		t.Errorf("unexpected survivors: %+v", got)
	}
}

// Unparseable JSON must be reported as "no list" so the fallback path runs.
func TestResourceEntriesBadJSONFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, clientResourcesFile),
		[]byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resourceEntries(dir); got != nil {
		t.Errorf("want nil for unparseable file, got %+v", got)
	}
}

// The category extractor handles both official shapes.
func TestEntryCategory(t *testing.T) {
	cases := map[string]string{
		"$azhash$9$7$395$2a544b966565ede4":   "az",
		"$cvhash$1480$dac1efd2859ad69c":      "cv",
		"$l2dhash$1590$772ece9edca7abb6":     "l2d",
		"$paintinghash$1800$2010b7cb86d14d76": "painting",
		"$maphash$333$059dc192332c5f56":      "map",
		"malformed":                          "az",
	}
	for in, want := range cases {
		if got := entryCategory(in); got != want {
			t.Errorf("entryCategory(%q) = %q, want %q", in, got, want)
		}
	}
}
