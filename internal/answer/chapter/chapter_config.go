package chapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/orm"
)

const (
	chapterTemplateCategory     = "sharecfgdata/chapter_template.json"
	chapterTemplateLoopCategory = "sharecfgdata/chapter_template_loop.json"
	chapterAutoStatisticsCat    = "ShareCfg/chapter_auto_statistics.json"
	itemDataStatsCategory       = "sharecfgdata/item_data_statistics.json"
	benefitBuffCategory         = "ShareCfg/benefit_buff_template.json"
	friendlyDataCategory        = "ShareCfg/friendly_data_template.json"
	friendlyDataShareCategory   = "sharecfgdata/friendly_data_template.json"
)

// chapterAutoStatistics 官服「自动战斗统计表」。
//   - boss_expedition_id：BOSS 舰队的候选 id。**1-15 章只有 1 个 BOSS**（打掉就整图结算，
//     多条候选是同一个 BOSS 每次刷新时可在随机点位/编成出现的候选）；
//   - **16 章才是多 BOSS**（16-x = 4 个，需按序列反复击破）；
//   - oil_limit = 锁油上限（9 章之后的锁油机制，9-1=182 … 16-1=355）。
type chapterAutoStatistics struct {
	ID               uint32   `json:"id"`
	OilLimit         uint32   `json:"oil_limit"`
	EnemyTimes       uint32   `json:"enemy_times"`
	BossExpeditionID []uint32 `json:"boss_expedition_id"`
}

type chapterTemplate struct {
	ID                 uint32       `json:"id"`
	Map                uint32       `json:"map"`
	Grids              [][]any      `json:"grids"`
	BoxList            [][]any      `json:"box_list"`
	RandomBoxList      []uint32     `json:"random_box_list"`
	LandBased          chapter2DAny `json:"land_based"`
	FriendlyID         uint32       `json:"friendly_id"`
	AmmoTotal          uint32       `json:"ammo_total"`
	AmmoSubmarine      uint32       `json:"ammo_submarine"`
	GroupNum           uint32       `json:"group_num"`
	SubmarineNum       uint32       `json:"submarine_num"`
	SupportGroupNum    uint32       `json:"support_group_num"`
	IsAmbush           uint32       `json:"is_ambush"`
	InvestigationRatio uint32       `json:"investigation_ratio"`
	AvoidRatio         uint32       `json:"avoid_ratio"`
	AmbushRatioExtra   [][]int32    `json:"ambush_ratio_extra"`
	ChapterStrategy    []uint32     `json:"chapter_strategy"`
	BossExpeditionID   []uint32     `json:"boss_expedition_id"`
	EnemyRefresh       []uint32     `json:"enemy_refresh"`
	EliteRefresh       []uint32     `json:"elite_refresh"`
	AiRefresh          []uint32     `json:"ai_refresh"`
	BoxRefresh         []uint32     `json:"box_refresh"`
	BossRefresh        uint32       `json:"boss_refresh"`
	ExpeditionWeight   [][]any      `json:"expedition_id_weight_list"`
	EliteExpeditions   []uint32     `json:"elite_expedition_list"`
	AmbushExpeditions  []uint32     `json:"ambush_expedition_list"`
	GuarderExpeditions []uint32     `json:"guarder_expedition_list"`
	Awards             [][]uint32   `json:"awards"`
	StarRequire1       uint32       `json:"star_require_1"`
	StarRequire2       uint32       `json:"star_require_2"`
	StarRequire3       uint32       `json:"star_require_3"`
	Num1               uint32       `json:"num_1"`
	Num2               uint32       `json:"num_2"`
	Num3               uint32       `json:"num_3"`
	ProgressBoss       uint32       `json:"progress_boss"`
	Oil                uint32       `json:"oil"`
	Time               uint32       `json:"time"`
}

// UnmarshalJSON 先把"空表"改写成空数组，再走默认解码。
// 原因：Lua 空表经 tools/lua2json.py 一律写成 {}（转换器分不出空数组和空表）。实测 9.7 的
// chapter_template.json：box_list 1012 行是 {}、ambush_ratio_extra 936 行、land_based 834 行、
// chapter_strategy 527 行、ambush_expedition_list 也是 {} …… 而 encoding/json 把对象解进切片字段
// 会直接报错，调用方拿到 error 就 client.CloseWithError 把整条连接 reset（实测一进关卡 SC_13102 掉线）。
// 这里用反射遍历本结构体的**切片字段**，只把对应的空对象值改写成 []：
//   - 不写死键名（第一版写死了，漏掉 ambush_expedition_list，第二次实机又掉线）；
//   - 只看以 { 开头的值 ⇒ 字符串形态的 land_based（190 行）与真正的对象字段不受影响。
func (t *chapterTemplate) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	typ := reflect.TypeOf(chapterTemplate{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		kind := field.Type.Kind()
		if kind != reflect.Slice && kind != reflect.Array {
			continue
		}
		if raw, ok := fields[key]; ok && strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
			fields[key] = json.RawMessage("[]")
		}
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain chapterTemplate // 别名：避免递归调用本方法
	return json.Unmarshal(normalized, (*plain)(t))
}

type chapter2DAny [][]any

func (value *chapter2DAny) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*value = chapter2DAny{}
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			*value = chapter2DAny{}
			return nil
		}
		return fmt.Errorf("invalid chapter matrix string: %s", text)
	}
	var entries [][]any
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	*value = chapter2DAny(entries)
	return nil
}

type itemDataStatisticsEntry struct {
	ID       uint32          `json:"id"`
	UsageArg json.RawMessage `json:"usage_arg"`
}

type benefitBuffEntry struct {
	ID               uint32 `json:"id"`
	BenefitType      string `json:"benefit_type"`
	BenefitEffect    string `json:"benefit_effect"`
	BenefitCondition string `json:"benefit_condition"`
}

type friendlyDataEntry struct {
	ID uint32 `json:"id"`
	HP uint32 `json:"hp"`
}

func loadChapterTemplate(chapterID uint32, loopFlag uint32) (*chapterTemplate, error) {
	category := chapterTemplateCategory
	if loopFlag != 0 {
		category = chapterTemplateLoopCategory
	}
	entry, err := orm.GetConfigEntry(category, fmt.Sprintf("%d", chapterID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var template chapterTemplate
	if err := json.Unmarshal(entry.Data, &template); err != nil {
		return nil, err
	}
	return &template, nil
}

// loadChapterAutoStatistics 读取官服自动战斗统计表（BOSS 舰队序列 / 锁油上限）。
// 查不到时返回 (nil, nil)，调用方回退到模板里的单个 boss_expedition_id。
func loadChapterAutoStatistics(chapterID uint32) (*chapterAutoStatistics, error) {
	entry, err := orm.GetConfigEntry(chapterAutoStatisticsCat, fmt.Sprintf("%d", chapterID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var stats chapterAutoStatistics
	if err := json.Unmarshal(entry.Data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func loadItemUsageArg(itemID uint32) ([]uint32, error) {
	entry, err := orm.GetConfigEntry(itemDataStatsCategory, fmt.Sprintf("%d", itemID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var stats itemDataStatisticsEntry
	if err := json.Unmarshal(entry.Data, &stats); err != nil {
		return nil, err
	}
	return decodeUsageArgUint32(stats.UsageArg)
}

func calculateOperationItemCostRate(itemID uint32) (float64, error) {
	if itemID == 0 {
		return 1, nil
	}
	ids, err := loadItemUsageArg(itemID)
	if err != nil {
		return 0, err
	}
	rate := 1.0
	for _, buffID := range ids {
		entry, err := loadBenefitBuff(buffID)
		if err != nil {
			return 0, err
		}
		if entry == nil || entry.BenefitType != "more_oil" {
			continue
		}
		effect, err := strconv.ParseFloat(entry.BenefitEffect, 64)
		if err != nil {
			continue
		}
		rate += effect * 0.01
	}
	return math.Max(1, rate), nil
}

func findOperationBuffID(itemID uint32) (uint32, error) {
	if itemID == 0 {
		return 0, nil
	}
	entries, err := orm.ListConfigEntries(benefitBuffCategory)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		var buff benefitBuffEntry
		if err := json.Unmarshal(entry.Data, &buff); err != nil {
			return 0, err
		}
		if buff.BenefitType != "desc" {
			continue
		}
		condition, err := strconv.ParseUint(buff.BenefitCondition, 10, 32)
		if err != nil {
			continue
		}
		if uint32(condition) == itemID {
			return buff.ID, nil
		}
	}
	return 0, nil
}

func loadBenefitBuff(buffID uint32) (*benefitBuffEntry, error) {
	entry, err := orm.GetConfigEntry(benefitBuffCategory, fmt.Sprintf("%d", buffID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var buff benefitBuffEntry
	if err := json.Unmarshal(entry.Data, &buff); err != nil {
		return nil, err
	}
	return &buff, nil
}

func loadFriendlyData(friendlyID uint32) (*friendlyDataEntry, error) {
	for _, category := range []string{friendlyDataCategory, friendlyDataShareCategory} {
		entry, err := orm.GetConfigEntry(category, fmt.Sprintf("%d", friendlyID))
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				continue
			}
			return nil, err
		}
		var friendly friendlyDataEntry
		if err := json.Unmarshal(entry.Data, &friendly); err != nil {
			return nil, err
		}
		return &friendly, nil
	}
	return nil, nil
}

func decodeUsageArgUint32(raw json.RawMessage) ([]uint32, error) {
	normalized, err := normalizeChapterUsageArg(raw)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	var ids []uint32
	if err := json.Unmarshal(normalized, &ids); err == nil {
		return ids, nil
	}
	var generic []any
	if err := json.Unmarshal(normalized, &generic); err != nil {
		return nil, err
	}
	ids = make([]uint32, 0, len(generic))
	for _, value := range generic {
		switch typed := value.(type) {
		case float64:
			ids = append(ids, uint32(typed))
		case string:
			parsed, err := strconv.ParseUint(typed, 10, 32)
			if err != nil {
				continue
			}
			ids = append(ids, uint32(parsed))
		}
	}
	return ids, nil
}

func normalizeChapterUsageArg(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			text = "[]"
		}
		if !json.Valid([]byte(text)) {
			return nil, fmt.Errorf("invalid usage_arg: %s", text)
		}
		return json.RawMessage([]byte(text)), nil
	}
	return raw, nil
}
