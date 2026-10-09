#!/usr/bin/env python3
"""Can the client actually resolve everything the official-save import wrote?

`cmd/importsave` copies the official login burst straight into the player tables:
`owned_ships.id` is the official *instance* id, `owned_ships.ship_id` is the *template*
id (e.g. 101994), and `ships.template_id` is what joins back to it. The client then
re-keys that ship by `group_type` (e.g. 10126) to build its collection.

Two numbers decide whether the client survives the login-time painting check
(LoginMediator:checkPaintingRes -> PaintingGroupConst:GetPaintingNameListInLogin ->
CollectionProxy:getGroups -> pg.ship_data_group[id]):

  * every owned ship's `ship_id` must exist in `ships`
        else the ship is one the client does not know
  * that `ships` row must have a non-NULL `group_type`
        else the collection entry resolves through a nil index and the main menu dies

The second is deliberately NOT the importer's job: `ships.group_type` is filled at server
boot by `misc.BackfillShipGroupTypes` from `<BELFAST_DATA_DIR>/ship_group_types.json`.
So a freshly imported account is only valid after a restart, and a template absent from
that mapping is a real (if quiet) hole - the boot log reports the total as
"N ships have no group_type ... will be skipped in SC_17001".

    python check-save-import.py [commander_id]      # default 2890086143

Exit code 1 when the account holds ships the client cannot resolve.
"""
from __future__ import annotations

import subprocess
import sys

CID = sys.argv[1] if len(sys.argv) > 1 else "2890086143"

# Postgres lives in WSL; psql is not on PATH. Same shape as the other tools/*.py checks.
PSQL_SH = r"""
export LD_LIBRARY_PATH="$HOME/pg/usr/lib/x86_64-linux-gnu"
BIN=$(ls -d "$HOME"/pg/usr/lib/postgresql/*/bin | head -1)
"$BIN/psql" -h 127.0.0.1 -U belfast -d belfast -t -A -F'|' -c "$SQL"
"""


def psql(sql: str) -> list[list[str]]:
    script = PSQL_SH.replace("$SQL", sql.replace('"', '\\"'))
    r = subprocess.run(["wsl", "-d", "Ubuntu-26.04", "-u", "niu", "-e", "bash", "-c", script],
                       capture_output=True, text=True, encoding="utf-8", errors="replace")
    out = ((r.stdout or "") + (r.stderr or "")).strip()
    if "ERROR" in out:
        print("  SQL 错误: " + out[:250])
        return []
    return [line.split("|") for line in out.splitlines() if line.strip()]


def scalar(sql: str) -> int:
    rows = psql(sql)
    try:
        return int(rows[0][0])
    except (IndexError, ValueError):
        return -1


print(f"commander_id = {CID}")

print("\n=== 导入器写的那几张表 ===")
for name, sql in (
    ("owned_ships", f"select count(*) from owned_ships where owner_id={CID}"),
    ("commander_items", f"select count(*) from commander_items where commander_id={CID}"),
    ("owned_equipments", f"select count(*) from owned_equipments where commander_id={CID}"),
    ("commander_tasks", f"select count(*) from commander_tasks where commander_id={CID}"),
):
    print(f"  {name:<18} {scalar(sql)}")

print("\n=== ships 配置表（group_type 回填的受体）===")
rows = psql("select count(*), count(group_type) from ships")
if rows:
    total, filled = rows[0]
    print(f"  行数 {total}，其中 group_type 非空 {filled}")
    missing = int(total) - int(filled) if total.isdigit() and filled.isdigit() else -1
    if missing > 0:
        print(f"  ⇒ {missing} 个模板不在 ship_group_types.json 里（启动日志会报同样的数）")

print("\n=== 不变量 1：owned_ships.ship_id 必须能在 ships 里查到 ===")
unknown = scalar(f"""
select count(*) from owned_ships o
left join ships s on s.template_id = o.ship_id
where o.owner_id={CID} and s.template_id is null
""")
print(f"  查不到的船: {unknown}")
if unknown:
    for r in psql(f"""
select o.ship_id, count(*) from owned_ships o
left join ships s on s.template_id = o.ship_id
where o.owner_id={CID} and s.template_id is null
group by 1 order by 1 limit 10"""):
        print(f"    tpl={r[0]}  x{r[1]}")

print("\n=== 不变量 2：查得到的那行必须有 group_type（客户端按它编目）===")
nogroup = scalar(f"""
select count(*) from owned_ships o
join ships s on s.template_id = o.ship_id
where o.owner_id={CID} and s.group_type is null
""")
print(f"  没有 group_type 的船: {nogroup}")
if nogroup:
    for r in psql(f"""
select o.ship_id, s.name, count(*) from owned_ships o
join ships s on s.template_id = o.ship_id
where o.owner_id={CID} and s.group_type is null
group by 1, 2 order by 3 desc, 1 limit 12"""):
        print(f"    tpl={r[0]}  {r[1][:24]:<24} x{r[2]}")
    print("  ⇒ 客户端图鉴里这些船会被跳过；启动日志同样会报这个数（正常情况为 0）")

broken = unknown > 0 or nogroup > 0
print("\n结论: " + ("有船无法被客户端解析，先跑数据升级 + 重启服务端回填 group_type"
                    if broken else "全部可解析 OK"))
sys.exit(1 if broken else 0)
