package misc

import (
	"os"
	"strings"
	"testing"
)

// TestLocalHashesAgainstRealTree runs the fingerprint builder over the actual data
// directory when BELFAST_REAL_DATA_DIR is set. The unit tests use tiny temp trees, so
// this is the only check that the shape holds for the real thing (627 files, ~70 MB).
//
//	powershell -c "$env:BELFAST_REAL_DATA_DIR='E:\Agent工作区\apkwork\belfast-data-97'; \
//	  go test ./internal/misc/ -run TestLocalHashesAgainstRealTree -v"
func TestLocalHashesAgainstRealTree(t *testing.T) {
	dir := strings.TrimSpace(os.Getenv("BELFAST_REAL_DATA_DIR"))
	if dir == "" {
		t.Skip("BELFAST_REAL_DATA_DIR not set")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("BELFAST_REAL_DATA_DIR=%s: %v", dir, err)
	}

	t.Setenv("BELFAST_DATA_DIR", dir)
	withVersions(t, "9.7.395")

	hashes := localHashes("CN")
	if hashes == nil {
		t.Fatal("localHashes returned nil for the real data dir")
	}

	t.Logf("entries: %d (+trailers added by updateVersions)", len(hashes))
	for _, h := range hashes {
		t.Logf("  %-10s %s", h.Category, h.Hash)
	}

	// When the recorded official list is present it must be used verbatim: those values
	// address index files on the CDN, so any rewriting breaks the client.
	if recorded := resourceEntries(dir); len(recorded) > 0 {
		t.Logf("using recorded %s (%d entries)", clientResourcesFile, len(recorded))
		if len(hashes) != len(recorded) {
			t.Fatalf("got %d entries, want the %d recorded ones", len(hashes), len(recorded))
		}
		for i, want := range recorded {
			if hashes[i].Hash != want {
				t.Errorf("entry %d = %q, want verbatim %q", i, hashes[i].Hash, want)
			}
		}
		return
	}

	t.Logf("no %s in %s; exercising the derived fallback", clientResourcesFile, dir)
	if hashes[0].Category != azMainCategory || !mainShape.MatchString(hashes[0].Hash) {
		t.Errorf("main entry malformed: %+v", hashes[0])
	}
	if !strings.Contains(hashes[0].Hash, "$9$7$395$") {
		t.Errorf("main entry must carry 9.7.395, got %s", hashes[0].Hash)
	}
	for i, h := range hashes[1:] {
		if !entryShape.MatchString(h.Hash) {
			t.Errorf("entry %d (%s) malformed: %s", i+1, h.Category, h.Hash)
		}
	}
	// The real tree must not collapse every category to the same digest.
	seen := map[string]bool{}
	for _, h := range hashes[1:] {
		seen[strings.SplitN(h.Hash, "$", 4)[3]] = true
	}
	if len(seen) != len(strings.Split(resourceCategories, ",")) {
		t.Errorf("categories share digests: %d distinct, want %d",
			len(seen), len(strings.Split(resourceCategories, ",")))
	}
}
