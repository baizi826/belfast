package misc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ggmolly/belfast/internal/logger"
)

// The SC_10801 version list describes the client's *resource* bundles, not our game
// data. The official answer (captured 2026-10-09, client build 9.7.394) looks like:
//
//	https://pkg.biligame.com/games/blhx_9.7.25_...apk
//	$azhash$9$7$394$adef01e2a471c5fc      main bundle; carries major/minor/build
//	$cvhash$1480$eebfc95fa1ad1e6e         1480 live2d/painting files
//	$l2dhash$1590$4cc2c919a8fa6178
//	$pichash$1312$8a012342ed5fa1ff
//	$bgmhash$1300$d10f89b716830704
//	$paintinghash$1796$07951b954f508729
//	$mangahash$1341$0213d27fad3637ae
//	$cipherhash$1304$a289769c12c76172
//	$dormhash$1550$67c2cfb8a895a2d7
//	$maphash$332$6f4684d8b0991599
//	count-2
//	dTag-1
//
// Two fixed shapes: `$<cat>hash$<n>$<hash>` for everything but the main bundle, and
// `$azhash$<major>$<minor>$<build>$<hash>` for it. Both trailers are literal.
//
// ponytail: we can derive the version numbers (from versions.json) but NOT the
// per-category counts or hashes - those fingerprint client assets (live2d / paintings
// / bgm / manga) that the server does not ship. Measured 2026-10-09: H is not any
// digest of the index file (crc64/sha1/sha256/sha512/md5 and byte-swapped variants all
// differ) and the CDN serves no directory listing, so the list cannot be computed and
// must be recorded once. It is recorded in client_resources.json and treated as DATA,
// exactly like versions.json and the mirrored Lua tables - the running server still
// constructs its own packet and never needs the capture. Refresh with
// tools/refresh-client-resources.py (CS_10800 is plain protobuf on port 80, so no CA
// and no MITM are needed). Fallback when the file is absent: derive from our own data
// tree - deterministic, but the client will fail with "Hash文件校验失败" because those
// values point at index files the CDN does not have.
const (
	// azMainCategory is the one entry that carries the client build number.
	azMainCategory = "az"

	// resourceCategories are the remaining bundle names the official packet carries,
	// in the order it lists them. The order is part of the contract.
	resourceCategories = "cv,l2d,pic,bgm,painting,manga,cipher,dorm,map"

	// clientResourcesFile holds the recorded official index list.
	clientResourcesFile = "client_resources.json"
)

// literal trailers are appended by the caller (answer.updateVersions), not here - this
// function produces only the `$...hash$...` entries the official packet carries.

// localHashes builds the SC_10801 fingerprint entries from BELFAST_DATA_DIR.
//
// Returns nil when no data directory is configured - the caller then sends an empty
// version list rather than reaching out to the official server.
func localHashes(region string) HashMap {
	dir := belfastDataDir()
	if dir == "" {
		return nil
	}

	// Preferred: the recorded official index list. These entries describe the client's
	// asset bundles, which the server does not ship, so they cannot be derived - see
	// the note on clientResourcesFile.
	if entries := resourceEntries(dir); len(entries) > 0 {
		hashes := make(HashMap, 0, len(entries))
		for _, entry := range entries {
			hashes = append(hashes, GameChecksum{Category: entryCategory(entry), Hash: entry})
		}
		return hashes
	}

	treeHash, fileCount, err := treeFingerprint(dir, region)
	if err != nil {
		logger.LogEvent("GameUpdate", "LocalHashes",
			fmt.Sprintf("cannot fingerprint %s: %v", dir, err), logger.LOG_LEVEL_ERROR)
		return nil
	}

	logger.LogEvent("GameUpdate", "LocalHashes",
		fmt.Sprintf("%s has no %s; falling back to a derived list the client may reject",
			dir, clientResourcesFile), logger.LOG_LEVEL_WARN)

	major, minor, build := splitVersion(azurLaneVersions[region].Version)

	hashes := make(HashMap, 0, 1+len(strings.Split(resourceCategories, ",")))
	hashes = append(hashes, GameChecksum{
		Category: azMainCategory,
		Hash: fmt.Sprintf("$%shash$%s$%s$%s$%s",
			azMainCategory, major, minor, build, shortHash(treeHash)),
	})

	// Per-category entries. The server has no per-category assets, so the count is the
	// data tree's own file count and each hash is salted with the category name -
	// stable across runs, and different per category so the list is not degenerate.
	for _, cat := range strings.Split(resourceCategories, ",") {
		salted := fmt.Sprintf("%s\x00%s\x00%s", cat, region, treeHash)
		digest := sha256.Sum256([]byte(salted))
		hashes = append(hashes, GameChecksum{
			Category: cat,
			Hash: fmt.Sprintf("$%shash$%d$%s",
				cat, fileCount, shortHash(hex.EncodeToString(digest[:]))),
		})
	}

	return hashes
}

// resourceEntries reads the recorded official index list. Returns nil when the file is
// absent so the caller can fall back.
func resourceEntries(dir string) []string {
	path := filepath.Join(dir, clientResourcesFile)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		Entries []string `json:"entries"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		logger.LogEvent("GameUpdate", "ClientResources",
			fmt.Sprintf("cannot parse %s: %v", path, err), logger.LOG_LEVEL_ERROR)
		return nil
	}
	out := make([]string, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		// Guard the shape: everything downstream assumes `$<cat>hash$...`.
		if strings.HasPrefix(e, "$") && strings.Contains(e, "hash$") {
			out = append(out, e)
		}
	}
	return out
}

// entryCategory pulls the bundle name out of `$<cat>hash$...`. The main bundle uses
// `$azhash$<major>$<minor>$<build>$<h>`, everything else `$<cat>hash$<n>$<h>`.
func entryCategory(entry string) string {
	rest := strings.TrimPrefix(entry, "$")
	if i := strings.Index(rest, "hash$"); i > 0 {
		return rest[:i]
	}
	return azMainCategory
}

// treeFingerprint hashes every file under <dir>/<region> (and the region-less root
// files) in sorted order, so the result depends only on the data, not on readdir order.
func treeFingerprint(dir, region string) (string, int, error) {
	paths := make([]string, 0, 1024)
	root := filepath.Join(dir, region)
	for _, base := range []string{root, dir} {
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			// Skip the loose root once we are already under <region>.
			if base == dir && strings.HasPrefix(path, root+string(os.PathSeparator)) {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return "", 0, err
		}
	}
	sort.Strings(paths)

	sum := sha256.New()
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return "", 0, err
		}
		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(sum, "%s\n", filepath.ToSlash(rel))
		sum.Write(body)
	}
	return hex.EncodeToString(sum.Sum(nil)), len(paths), nil
}

// splitVersion turns "9.7.395" into its three numeric parts. A malformed version
// yields zeros rather than an error - the client is given a well-formed shape either way.
func splitVersion(version string) (string, string, string) {
	parts := strings.Split(version, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	for i, p := range parts[:3] {
		if _, err := strconv.Atoi(p); err != nil {
			logger.LogEvent("GameUpdate", "LocalHashes",
				fmt.Sprintf("version %q has non-numeric part %q, using 0", version, p), logger.LOG_LEVEL_WARN)
			parts[i] = "0"
		}
	}
	return parts[0], parts[1], parts[2]
}

// shortHash keeps the first 8 bytes of a hex digest - the width the official packet uses.
func shortHash(hexDigest string) string {
	if len(hexDigest) > 16 {
		return hexDigest[:16]
	}
	return hexDigest
}
