package guildshop

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/orm"
	rngutil "github.com/ggmolly/belfast/internal/rng"
	"github.com/ggmolly/belfast/internal/shopreset"
)

const (
	guildStoreConfigCategory = "ShareCfg/guild_store.json"
	guildSetConfigCategory   = "ShareCfg/guildset.json"
)

type StoreEntry struct {
	ID                 uint32 `json:"id"`
	Weight             uint32 `json:"weight"`
	GoodsPurchaseLimit uint32 `json:"goods_purchase_limit"`
}

type SetEntry struct {
	Key      string     `json:"key"`
	KeyValue uint32     `json:"key_value"`
	KeyArgs  SetKeyArgs `json:"key_args"`
}

// SetKeyArgs 容忍 []uint32 与字符串两种形态。
// 9.7 的 ShareCfg/guildset.json 里 key_args 有的行是字符串（Lua 空表/字符串经转换器落成 "" 或 "50,80"），
// 直接解进 []uint32 会报 "cannot unmarshal string into ... []uint32"，调用方把 error 往上抛 ⇒
// 整条连接被 reset（实测 SC_60034 一进大舰队商店就掉线）。字符串里抽出数字，抽不到就当空。
type SetKeyArgs []uint32

func (a *SetKeyArgs) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*a = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var v []uint32
		if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
			return err
		}
		*a = v
		return nil
	}
	var s string
	if err := json.Unmarshal([]byte(trimmed), &s); err != nil {
		*a = nil // 其它标量形态一律当空，不再让整条连接陪葬
		return nil
	}
	var out []uint32
	for _, field := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		if v, err := strconv.ParseUint(field, 10, 32); err == nil {
			out = append(out, uint32(v))
		}
	}
	*a = out
	return nil
}

type Config struct {
	StoreEntries []StoreEntry
	GoodsCount   uint32
	RefreshLimit uint32
	RefreshCosts []uint32
}

func (c *Config) CanManualRefresh(currentCount uint32) bool {
	if c == nil || c.RefreshLimit == 0 {
		return true
	}
	return currentCount < c.RefreshLimit
}

func (c *Config) RefreshCost(nextCount uint32) uint32 {
	if c == nil || nextCount == 0 || len(c.RefreshCosts) == 0 {
		return 0
	}
	index := int(nextCount - 1)
	if index >= len(c.RefreshCosts) {
		return c.RefreshCosts[len(c.RefreshCosts)-1]
	}
	return c.RefreshCosts[index]
}

func LoadConfig() (*Config, error) {
	storeEntries, err := orm.ListConfigEntries(guildStoreConfigCategory)
	if err != nil {
		return nil, err
	}
	entries := make([]StoreEntry, 0, len(storeEntries))
	for _, entry := range storeEntries {
		var store StoreEntry
		if err := json.Unmarshal(entry.Data, &store); err != nil {
			return nil, err
		}
		if store.ID == 0 {
			continue
		}
		entries = append(entries, store)
	}
	goodsCountEntry, err := getGuildSetEntry("store_goods_quantity")
	if err != nil {
		return nil, err
	}
	storeRefreshEntry, err := getGuildSetEntry("store_refresh")
	if err != nil {
		return nil, err
	}
	storeResetCostEntry, err := getGuildSetEntry("store_reset_cost")
	if err != nil {
		return nil, err
	}

	goodsCount := goodsCountEntry.KeyValue
	refreshLimit := uint32(1)
	if len(storeRefreshEntry.KeyArgs) >= 2 && storeRefreshEntry.KeyArgs[1] > 0 {
		refreshLimit = storeRefreshEntry.KeyArgs[1]
	} else if storeRefreshEntry.KeyValue > 0 {
		refreshLimit = storeRefreshEntry.KeyValue
	}
	refreshCosts := append([]uint32{}, storeResetCostEntry.KeyArgs...)
	if len(refreshCosts) == 0 {
		if storeResetCostEntry.KeyValue > 0 {
			refreshCosts = []uint32{storeResetCostEntry.KeyValue}
		} else {
			refreshCosts = []uint32{0}
		}
	}
	if goodsCount == 0 {
		goodsCount = 10
	}
	return &Config{
		StoreEntries: entries,
		GoodsCount:   goodsCount,
		RefreshLimit: refreshLimit,
		RefreshCosts: refreshCosts,
	}, nil
}

func EnsureState(commanderID uint32, now time.Time, config *Config) (*orm.GuildShopState, []orm.GuildShopGood, error) {
	state, err := orm.GetGuildShopState(commanderID)
	if err != nil {
		if !db.IsNotFound(err) {
			return nil, nil, err
		}
		state = &orm.GuildShopState{
			CommanderID:     commanderID,
			RefreshCount:    0,
			NextRefreshTime: nextDailyReset(now),
		}
		if err := orm.CreateGuildShopState(*state); err != nil {
			return nil, nil, err
		}
		goods, err := RefreshGoods(commanderID, now, config, RefreshOptions{
			RefreshCount:    0,
			NextRefreshTime: state.NextRefreshTime,
		})
		if err != nil {
			return nil, nil, err
		}
		return state, goods, nil
	}
	goods, err := LoadGoods(commanderID)
	if err != nil {
		return nil, nil, err
	}
	return state, goods, nil
}

func RefreshIfNeeded(commanderID uint32, now time.Time, config *Config) (*orm.GuildShopState, []orm.GuildShopGood, error) {
	state, goods, err := EnsureState(commanderID, now, config)
	if err != nil {
		return nil, nil, err
	}
	if now.Unix() >= int64(state.NextRefreshTime) || len(goods) == 0 {
		goods, err = RefreshGoods(commanderID, now, config, RefreshOptions{
			RefreshCount:    0,
			NextRefreshTime: nextDailyReset(now),
		})
		if err != nil {
			return nil, nil, err
		}
		state, err = orm.GetGuildShopState(commanderID)
		if err != nil {
			return nil, nil, err
		}
	}
	return state, goods, nil
}

type RefreshOptions struct {
	RefreshCount    uint32
	NextRefreshTime uint32
}

func RefreshGoods(commanderID uint32, now time.Time, config *Config, options RefreshOptions) ([]orm.GuildShopGood, error) {
	goods := buildGoods(commanderID, config, refreshSeed(commanderID, now, options.RefreshCount))
	if err := orm.RefreshGuildShopGoods(commanderID, goods, options.RefreshCount, options.NextRefreshTime); err != nil {
		return nil, err
	}
	return goods, nil
}

func LoadGoods(commanderID uint32) ([]orm.GuildShopGood, error) {
	return orm.LoadGuildShopGoods(commanderID)
}

func buildGoods(commanderID uint32, config *Config, seed uint64) []orm.GuildShopGood {
	if config == nil {
		return nil
	}
	entries := selectGoods(config.StoreEntries, int(config.GoodsCount), seed)
	goods := make([]orm.GuildShopGood, 0, len(entries))
	for i, entry := range entries {
		count := entry.GoodsPurchaseLimit
		if count == 0 {
			count = 1
		}
		goods = append(goods, orm.GuildShopGood{
			CommanderID: commanderID,
			Index:       uint32(i + 1),
			GoodsID:     entry.ID,
			Count:       count,
		})
	}
	return goods
}

func selectGoods(entries []StoreEntry, count int, seed uint64) []StoreEntry {
	if count <= 0 || len(entries) == 0 {
		return nil
	}
	if len(entries) <= count {
		return entries
	}
	pool := make([]StoreEntry, len(entries))
	copy(pool, entries)
	rng := rngutil.NewLockedRandFromSeed(seed)
	selected := make([]StoreEntry, 0, count)
	for len(selected) < count && len(pool) > 0 {
		total := uint32(0)
		for _, entry := range pool {
			weight := entry.Weight
			if weight == 0 {
				weight = 1
			}
			total += weight
		}
		roll := rng.Uint32N(total)
		idx := 0
		for i, entry := range pool {
			weight := entry.Weight
			if weight == 0 {
				weight = 1
			}
			if roll < weight {
				idx = i
				break
			}
			roll -= weight
		}
		selected = append(selected, pool[idx])
		pool = append(pool[:idx], pool[idx+1:]...)
	}
	return selected
}

func getGuildSetEntry(key string) (*SetEntry, error) {
	entry, err := orm.GetConfigEntry(guildSetConfigCategory, key)
	if err != nil {
		return nil, err
	}
	var setEntry SetEntry
	if err := json.Unmarshal(entry.Data, &setEntry); err != nil {
		return nil, err
	}
	return &setEntry, nil
}

func nextDailyReset(now time.Time) uint32 {
	window, err := shopreset.DailyWindow(now)
	if err == nil {
		return uint32(window.End.Unix())
	}
	utc := now.UTC()
	next := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	return uint32(next.Unix())
}

func refreshSeed(commanderID uint32, now time.Time, refreshCount uint32) uint64 {
	window, err := shopreset.DailyWindow(now)
	if err != nil {
		return shopreset.DeterministicSeed(commanderID, uint32(now.UTC().Truncate(24*time.Hour).Unix()), refreshCount)
	}
	return shopreset.DeterministicSeed(commanderID, window.Key, refreshCount)
}
