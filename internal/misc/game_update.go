package misc

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ggmolly/belfast/internal/logger"
	"github.com/ggmolly/belfast/internal/region"
)

const (
	versionURL = "https://raw.githubusercontent.com/ggmolly/belfast-data/main/versions.json"
)

var (
	errRegionMismatch  = errors.New("Region mismatch")
	errVersionMismatch = errors.New("Version mismatch")
)

type VersionMap map[string]Version
type HashMap []GameChecksum

type Version struct {
	Region  string
	Version string
}

type GameChecksum struct {
	Category string
	Hash     string
}

type hashCache struct {
	Region  string
	Version string
	Hashes  HashMap
}

var azurLaneHashes HashMap
var azurLaneVersions VersionMap

func GetLatestVersions() VersionMap {
	return azurLaneVersions
}

func hashFromCache() (HashMap, error) {
	// use gob to read the cache file
	file, err := os.Open(".cached_hashes")
	if err != nil { // no cache
		return nil, err
	}
	defer file.Close()
	decoder := gob.NewDecoder(file)
	var cache hashCache
	err = decoder.Decode(&cache)
	if err != nil {
		logger.LogEvent("GameUpdate", "GetHashes", err.Error(), logger.LOG_LEVEL_ERROR)
		return nil, err
	}
	region := region.Current()
	if cache.Region != region {
		return nil, errRegionMismatch
	}
	if cache.Version != azurLaneVersions[region].Version {
		return nil, errVersionMismatch
	}
	return cache.Hashes, nil

}

func GetGameHashes() HashMap {
	return getGameHashes(false)
}

func GetGameHashesWithUpdate() HashMap {
	return getGameHashes(true)
}

func getGameHashes(triggerUpdate bool) HashMap {
	region := region.Current()
	version := azurLaneVersions[region].Version

	if azurLaneHashes != nil && azurLaneVersions[region].Version == version {
		return azurLaneHashes
	}

	// check if we have a cached version
	hashes, err := hashFromCache()
	if err == nil && azurLaneVersions[region].Version == version {
		azurLaneHashes = hashes
		return hashes
	}

	// No cache. Build the fingerprint from our own data tree instead of dialing the
	// official gateway: a private server must not require the official server to be
	// reachable, and its answer must not be a captured byte blob.
	azurLaneHashes = localHashes(region)
	if azurLaneHashes == nil {
		logger.LogEvent("GameUpdate", "GetHashes",
			"cannot build local hashes, SC_10801 will carry no version list", logger.LOG_LEVEL_ERROR)
		return nil
	}

	// Cache the hashes
	cache := hashCache{
		Region:  region,
		Version: azurLaneVersions[region].Version,
		Hashes:  azurLaneHashes,
	}
	file, err := os.OpenFile(".cached_hashes", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		logger.LogEvent("GameUpdate", "GetHashes", err.Error(), logger.LOG_LEVEL_ERROR)
		return azurLaneHashes
	}
	defer file.Close()
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(cache); err != nil {
		logger.LogEvent("GameUpdate", "GetHashes", err.Error(), logger.LOG_LEVEL_ERROR)
	}
	if triggerUpdate {
		go UpdateAllData(region)
	}
	return azurLaneHashes
}

func LastCacheUpdate() time.Time {
	file, err := os.Stat(".cached_hashes")
	if err != nil {
		return time.Time{}
	}
	return file.ModTime()
}

func LastCacheUpdateVersion() string {
	if azurLaneHashes == nil {
		return ""
	}
	region := region.Current()
	return azurLaneVersions[region].Version
}

func init() {
	// 下载最新版本号并解析成 map。设了 BELFAST_DATA_DIR 就从本地读，不走网络。
	var body io.ReadCloser
	if dir := belfastDataDir(); dir != "" {
		path := filepath.Join(dir, "versions.json")
		f, err := os.Open(path)
		if err != nil {
			logger.LogEvent("GameUpdate", "init", fmt.Sprintf("failed to open local versions.json: %s", err.Error()), logger.LOG_LEVEL_ERROR)
			return
		}
		body = f
	} else {
		resp, err := http.Get(versionURL)
		if err != nil {
			logger.LogEvent("GameUpdate", "init", fmt.Sprintf("failed to fetch versions: %s", err.Error()), logger.LOG_LEVEL_ERROR)
			return
		}
		body = resp.Body
	}
	defer body.Close()
	decoder := json.NewDecoder(body)
	deserializedMap := make(map[string]string)
	if err := decoder.Decode(&deserializedMap); err != nil {
		logger.LogEvent("GameUpdate", "init", fmt.Sprintf("failed to parse versions: %s", err.Error()), logger.LOG_LEVEL_ERROR)
		return
	}
	azurLaneVersions = make(VersionMap)
	for region, version := range deserializedMap {
		azurLaneVersions[region] = Version{
			Region:  region,
			Version: version,
		}
	}
}
