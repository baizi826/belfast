// cmd/importsave —— 把**官服抓包的登录 burst payload** 直接灌进 belfast 数据库。
//
// 为什么要有它：上游没有存档导入；而「一个屏卡住就补一个 handler」这条路永远收敛不了 ——
// 官服抓包是满级号（上百条船 / 上千道具装备 / 一堆任务），我们库里却是 15 条船 / 6 个道具
// ⇒ 登录 burst 里一大半是空壳 ⇒ 界面轮着卡。正确做法是**先有账号数据**。
//
// 输入：tools/bhx-mapdump.py 落盘的 req<reqcmd>_rep<cmd>_<n>.bin —— **纯 protobuf payload**
// （7 字节帧头与 zlib 已在抓包阶段处理掉）。
//
// 用法（WSL 里跑；Postgres 也在 WSL，所以 DSN 用 127.0.0.1 即可）：
//
//	go run ./cmd/importsave -dir /home/niu/belfast-import -commander 2890086143
//
// 可选：-dry-run 只统计不写库；-keep 不清空旧数据；-schema 指定库 schema（默认 belfast）。
//
// 设计：按「表」分组，而不是按包。例如船同时来自 12001(前 100 条) 与 12010(其余，多包)，
// 必须合并成一次「先清后插」，否则后一个包会把前一个包刚插的行删掉。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/protobuf"
)

var fileRe = regexp.MustCompile(`^req\d+_rep(\d+)_(\d+)\.bin$`)

// 包号 → 表分组
var groupOf = map[int]string{
	15001: "items",
	12001: "ships",
	12010: "ships",
	14001: "equips",
	20001: "tasks",
}

func main() {
	dir := flag.String("dir", "", "目录：内含 req*_rep<cmd>_<n>.bin")
	cid := flag.Uint64("commander", 0, "目标指挥官 id（owners/commander_id）")
	dsn := flag.String("dsn", os.Getenv("BELFAST_DSN"), "postgres DSN")
	schema := flag.String("schema", "belfast", "库 schema")
	dryRun := flag.Bool("dry-run", false, "只统计，不写库")
	keep := flag.Bool("keep", false, "不清空旧数据（默认先清后插）")
	flag.Parse()

	if *dir == "" || *cid == 0 || *dsn == "" {
		fmt.Fprintln(os.Stderr, "需要 -dir、-commander 和 -dsn（或 BELFAST_DSN 环境变量）")
		flag.PrintDefaults()
		os.Exit(2)
	}

	// 收集文件并按表分组
	files, err := filepath.Glob(filepath.Join(*dir, "*.bin"))
	if err != nil {
		fatal(err)
	}
	groups := map[string][][]byte{}
	skipped := map[int]int{}
	for _, f := range files {
		m := fileRe.FindStringSubmatch(filepath.Base(f))
		if m == nil {
			continue
		}
		cmd, _ := strconv.Atoi(m[1])
		g, ok := groupOf[cmd]
		if !ok {
			skipped[cmd]++
			continue
		}
		payload, err := os.ReadFile(f)
		if err != nil {
			fatal(err)
		}
		groups[g] = append(groups[g], payload)
	}
	fmt.Printf("读入 %d 个 payload；分组：", len(files))
	for _, g := range []string{"items", "ships", "equips", "tasks"} {
		fmt.Printf("%s=%d ", g, len(groups[g]))
	}
	fmt.Printf("；未映射的包 %d 种（不导入）\n", len(skipped))

	ctx := context.Background()
	store, err := db.InitDefaultStore(ctx, *dsn, *schema)
	if err != nil {
		fatal(err)
	}

	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	type result struct {
		group string
		info  string
	}
	var results []result
	for _, g := range []string{"items", "ships", "equips", "tasks"} {
		if len(groups[g]) == 0 {
			continue
		}
		var info string
		var err error
		switch g {
		case "items":
			info, err = importItems(ctx, tx, *cid, groups[g], *keep)
		case "ships":
			info, err = importShips(ctx, tx, *cid, groups[g], *keep)
		case "equips":
			info, err = importEquips(ctx, tx, *cid, groups[g], *keep)
		case "tasks":
			info, err = importTasks(ctx, tx, *cid, groups[g], *keep)
		}
		if err != nil {
			fatal(fmt.Errorf("%s: %w", g, err))
		}
		results = append(results, result{g, info})
	}

	if *dryRun {
		_ = tx.Rollback(ctx)
		fmt.Println("[-dry-run] 已回滚，什么都没写")
	} else if err := tx.Commit(ctx); err != nil {
		fatal(err)
	}

	for _, r := range results {
		fmt.Printf("  %-6s %s\n", r.group, r.info)
	}
}

// ---------------------------------------------------------------- items (SC_15001)

func importItems(ctx context.Context, tx pgx.Tx, cid uint64, payloads [][]byte, keep bool) (string, error) {
	counts := map[uint32]uint32{}
	misc := 0
	for _, p := range payloads {
		var msg protobuf.SC_15001
		if err := proto.Unmarshal(p, &msg); err != nil {
			return "", err
		}
		for _, it := range msg.GetItemList() {
			counts[it.GetId()] = it.GetCount()
		}
		for _, it := range msg.GetLimitList() {
			counts[it.GetId()] = it.GetCount()
		}
		misc += len(msg.GetItemMiscList())
	}
	if !keep {
		if _, err := tx.Exec(ctx, `DELETE FROM commander_items WHERE commander_id = $1`, cid); err != nil {
			return "", err
		}
	}
	for id, n := range counts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO commander_items (commander_id, item_id, count) VALUES ($1,$2,$3)`,
			cid, id, n); err != nil {
			return "", fmt.Errorf("item %d: %w", id, err)
		}
	}
	return fmt.Sprintf("%d 条道具（另有 %d 条 misc 未导入）", len(counts), misc), nil
}

// ---------------------------------------------------------------- ships (SC_12001 + SC_12010)

func importShips(ctx context.Context, tx pgx.Tx, cid uint64, payloads [][]byte, keep bool) (string, error) {
	ships := map[uint32]*protobuf.SHIPINFO{}
	equips, skills, strengths, transforms, shadows := 0, 0, 0, 0, 0
	for _, p := range payloads {
		var msg protobuf.SC_12001
		if err := proto.Unmarshal(p, &msg); err != nil {
			// 同一个 payload 可能是 SC_12010
			var d protobuf.SC_12010
			if err2 := proto.Unmarshal(p, &d); err2 != nil {
				return "", err
			}
			msg.Shiplist = d.GetShipList()
		}
		for _, s := range msg.GetShiplist() {
			ships[s.GetId()] = s
			equips += len(s.GetEquipInfoList())
			skills += len(s.GetSkillIdList())
			strengths += len(s.GetStrengthList())
			transforms += len(s.GetTransformList())
			shadows += len(s.GetSkinShadowList())
		}
	}
	// owned_ships has 28 columns and this importer writes 18 of them. The delete+insert
	// below resets everything else to its default, and one of those columns is
	// `is_secretary`: a re-import silently wiped the secretary assignment, PlayerInfo then
	// found no secretary and returned *without sending SC_11003*, and the client sat on the
	// loading screen forever. Stash the columns this importer does not own so the import
	// cannot destroy state it never imported.
	//
	// `deleted_at` is deliberately not preserved - a ship absent from the capture must come
	// back listed, and the capture is the authority on which ships the account owns.
	preserved := []string{
		"surplus_exp", "blueprint_flag",
		"state_info1", "state_info2", "state_info3", "state_info4",
		"is_secretary", "secretary_position", "secretary_phantom_id",
	}
	if _, err := tx.Exec(ctx, `
CREATE TEMP TABLE imported_ship_keep ON COMMIT DROP AS
SELECT id, `+strings.Join(preserved, ", ")+` FROM owned_ships WHERE owner_id = $1`, cid); err != nil {
		return "", err
	}

	if !keep {
		if _, err := tx.Exec(ctx, `DELETE FROM owned_ships WHERE owner_id = $1`, cid); err != nil {
			return "", err
		}
	}
	ids := make([]uint32, 0, len(ships))
	for id := range ships {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		s := ships[id]
		if _, err := tx.Exec(ctx, `
INSERT INTO owned_ships
  (owner_id, id, ship_id, level, exp, max_level, intimacy, energy, state, is_locked, propose,
   common_flag, activity_npc, custom_name, skin_id, proficiency, create_time, change_name_timestamp)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			cid, s.GetId(), s.GetTemplateId(), s.GetLevel(), s.GetExp(), s.GetMaxLevel(),
			s.GetIntimacy(), s.GetEnergy(), s.GetState(), s.GetIsLocked() != 0, s.GetPropose() != 0,
			s.GetCommonFlag() != 0, s.GetActivityNpc(), s.GetName(), s.GetSkinId(),
			s.GetProficiency() != 0,
			time.Unix(int64(s.GetCreateTime()), 0),
			time.Unix(int64(s.GetChangeNameTimestamp()), 0),
		); err != nil {
			return "", fmt.Errorf("ship %d: %w", id, err)
		}
	}
	// 顺带把 EquipList 里那种「无实例」的装备也记一笔（EQUIPSKIN_INFO 只有模板 id + 皮肤）
	// Put back the columns the importer does not own (see the stash above).
	sets := make([]string, len(preserved))
	for i, col := range preserved {
		sets[i] = col + " = k." + col
	}
	if _, err := tx.Exec(ctx, `
UPDATE owned_ships AS o SET `+strings.Join(sets, ", ")+`
FROM imported_ship_keep AS k
WHERE o.id = k.id AND o.owner_id = $1`, cid); err != nil {
		return "", err
	}

	children, err := importShipChildren(ctx, tx, cid, ships, keep)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d 条船（%s；未导入：技能 %d / 皮肤影 %d）",
		len(ships), children, skills, shadows), nil
}

// importShipChildren writes the three tables that hang off owned_ships.id from the
// nested SC_12001 / SC_12010 fields.
//
// Before this, the nested data was only *counted* — the dry run printed
// "未导入：装备位 6055 / 技能 2376 / 强化 1450 / 改造 339" and the values were dropped, which
// is why the client showed ships with no equipment and nothing to strengthen. Nothing else
// has to change to make them visible: orm/players_sqlc.go already attaches
// Equipments/Strengths/Transforms to the ship when the commander is loaded, and
// orm/adapters.go already emits them in SHIPINFO. The tables were simply empty.
//
// Empty slots are skipped. The server's own builder (buildEquipInfoList) always emits
// slotCount entries, filling unused ones with zeros, so a row with equip_id = 0 carries no
// information and would only inflate owned_equipments' counts.
func importShipChildren(ctx context.Context, tx pgx.Tx, cid uint64, ships map[uint32]*protobuf.SHIPINFO, keep bool) (string, error) {
	if !keep {
		// owned_ships has just been deleted and these FKs are ON DELETE CASCADE, so they
		// are already empty; the explicit DELETEs keep this right if that ever changes.
		for _, table := range []string{"owned_ship_equipments", "owned_ship_strengths", "owned_ship_transforms"} {
			if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE owner_id = $1", cid); err != nil {
				return "", err
			}
		}
	}

	ids := make([]uint32, 0, len(ships))
	for id := range ships {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var equipments, strengths, transforms int
	skippedSlots := 0
	for _, id := range ids {
		ship := ships[id]
		for i, e := range ship.GetEquipInfoList() {
			if e.GetId() == 0 && e.GetSkinId() == 0 {
				skippedSlots++
				continue
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO owned_ship_equipments (owner_id, ship_id, pos, equip_id, skin_id)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (owner_id, ship_id, pos)
DO UPDATE SET equip_id = EXCLUDED.equip_id, skin_id = EXCLUDED.skin_id`,
				cid, id, uint32(i+1), e.GetId(), e.GetSkinId()); err != nil {
				return "", fmt.Errorf("ship %d equipment pos %d: %w", id, i+1, err)
			}
			equipments++
		}
		for _, s := range ship.GetStrengthList() {
			if s.GetId() == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO owned_ship_strengths (owner_id, ship_id, strength_id, exp)
VALUES ($1,$2,$3,$4)
ON CONFLICT (owner_id, ship_id, strength_id)
DO UPDATE SET exp = EXCLUDED.exp`,
				cid, id, s.GetId(), s.GetExp()); err != nil {
				return "", fmt.Errorf("ship %d strength %d: %w", id, s.GetId(), err)
			}
			strengths++
		}
		for _, t := range ship.GetTransformList() {
			if t.GetId() == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO owned_ship_transforms (owner_id, ship_id, transform_id, level)
VALUES ($1,$2,$3,$4)
ON CONFLICT (owner_id, ship_id, transform_id)
DO UPDATE SET level = EXCLUDED.level`,
				cid, id, t.GetId(), t.GetLevel()); err != nil {
				return "", fmt.Errorf("ship %d transform %d: %w", id, t.GetId(), err)
			}
			transforms++
		}
	}
	return fmt.Sprintf("装备槽 %d / 强化 %d / 改造 %d（跳过空槽 %d）",
		equipments, strengths, transforms, skippedSlots), nil
}

// ---------------------------------------------------------------- equips (SC_14001)

func importEquips(ctx context.Context, tx pgx.Tx, cid uint64, payloads [][]byte, keep bool) (string, error) {
	counts := map[uint32]uint32{}
	spweapons := 0
	for _, p := range payloads {
		var msg protobuf.SC_14001
		if err := proto.Unmarshal(p, &msg); err != nil {
			return "", err
		}
		for _, e := range msg.GetEquipList() {
			counts[e.GetId()] = e.GetCount()
		}
		spweapons += len(msg.GetSpweaponList())
	}
	if !keep {
		if _, err := tx.Exec(ctx, `DELETE FROM owned_equipments WHERE commander_id = $1`, cid); err != nil {
			return "", err
		}
	}
	for id, n := range counts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO owned_equipments (commander_id, equipment_id, count) VALUES ($1,$2,$3)`,
			cid, id, n); err != nil {
			return "", fmt.Errorf("equip %d: %w", id, err)
		}
	}
	return fmt.Sprintf("%d 条装备（另有 %d 条特殊兵器未导入）", len(counts), spweapons), nil
}

// ---------------------------------------------------------------- tasks (SC_20001)

func importTasks(ctx context.Context, tx pgx.Tx, cid uint64, payloads [][]byte, keep bool) (string, error) {
	type row struct{ progress, accept, submit uint32 }
	tasks := map[uint32]row{}
	for _, p := range payloads {
		var msg protobuf.SC_20001
		if err := proto.Unmarshal(p, &msg); err != nil {
			return "", err
		}
		for _, t := range msg.GetInfo() {
			tasks[t.GetId()] = row{t.GetProgress(), t.GetAcceptTime(), t.GetSubmitTime()}
		}
	}
	if !keep {
		if _, err := tx.Exec(ctx, `DELETE FROM commander_tasks WHERE commander_id = $1`, cid); err != nil {
			return "", err
		}
	}
	ids := make([]uint32, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		t := tasks[id]
		if _, err := tx.Exec(ctx, `
INSERT INTO commander_tasks (commander_id, task_id, progress, accept_time, submit_time)
VALUES ($1,$2,$3,$4,$5)`, cid, id, t.progress, t.accept, t.submit); err != nil {
			return "", fmt.Errorf("task %d: %w", id, err)
		}
	}
	return fmt.Sprintf("%d 条任务", len(tasks)), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "!! "+err.Error())
	os.Exit(1)
}
