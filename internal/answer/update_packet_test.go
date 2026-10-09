package answer

import (
	"strings"
	"testing"

	"github.com/ggmolly/belfast/internal/misc"
)

// updateVersions owns the trailer contract. The official SC_10801 ends its version
// list with `count-2` then `dTag-1`; before this test existed the code appended only
// `dTag-1`, and a captured replay could accidentally supply the other.
func TestUpdateVersionsAppendsBothTrailers(t *testing.T) {
	saved := versions
	versions = nil
	defer func() { versions = saved }()

	fake := func() misc.HashMap {
		return misc.HashMap{
			{Category: "az", Hash: "$azhash$9$7$395$0123456789abcdef"},
			{Category: "cv", Hash: "$cvhash$10$fedcba9876543210"},
		}
	}

	got := updateVersions(fake)
	want := []string{
		"$azhash$9$7$395$0123456789abcdef",
		"$cvhash$10$fedcba9876543210",
		"count-2",
		"dTag-1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
	if n := strings.Count(strings.Join(got, ","), "dTag-1"); n != 1 {
		t.Errorf("dTag-1 appears %d times, want exactly 1", n)
	}
}

// The list is cached in a package var; a second call must reuse it, not double-append.
func TestUpdateVersionsCaches(t *testing.T) {
	saved := versions
	versions = nil
	defer func() { versions = saved }()

	calls := 0
	fake := func() misc.HashMap {
		calls++
		return misc.HashMap{{Category: "az", Hash: "$azhash$9$7$395$0123456789abcdef"}}
	}

	first := updateVersions(fake)
	second := updateVersions(fake)

	if calls != 1 {
		t.Errorf("hash source called %d times, want 1", calls)
	}
	if len(first) != len(second) {
		t.Errorf("cached call returned %d entries, want %d", len(second), len(first))
	}
}

// An empty hash source still yields the trailers, so the client sees a well-formed list.
func TestUpdateVersionsEmptySource(t *testing.T) {
	saved := versions
	versions = nil
	defer func() { versions = saved }()

	got := updateVersions(func() misc.HashMap { return nil })
	if len(got) != 2 || got[0] != "count-2" || got[1] != "dTag-1" {
		t.Errorf("got %v, want [count-2 dTag-1]", got)
	}
}
