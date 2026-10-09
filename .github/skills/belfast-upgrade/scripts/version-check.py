#!/usr/bin/env python3
"""Compare the upstream / mirror / built / database data versions of belfast.

`check-upgrade-coverage.py` only compares the local build against the frozen 9.6
baseline, so it happily reports "586 tables upgraded" while the build is still
weeks behind the live client. This script answers the question that has to be
settled before any private-server validation:

    is our data the SAME version as the official one right now?

`mark` in activity_template is the activity's release date (YYYYMMDD), so the
maximum mark is a direct "how new is this data" probe.

Layers, each independently observable:

  1. upstream  AzurLaneLuaScripts/CN/sharecfg/activity_template.lua   (live, daily)
  2. mirror    <work>/al-lua/CN/sharecfg/activity_template.lua        (mirrored copy)
  3. built     <work>/belfast-data-97/CN/ShareCfg/activity_template.json
  4. database  config_entries where category='ShareCfg/activity_template.json'

Environment (all optional, defaults match this machine):

  BELFAST_WORK_DIR   default E:\\Agent工作区\\apkwork
  BELFAST_WSL_DISTRO default Ubuntu-26.04
  BELFAST_PG_USER    default belfast
  BELFAST_PG_DB      default belfast

Usage:
  python version-check.py                 # full report
  python version-check.py --no-network    # skip the upstream fetch (offline)
  python version-check.py --json          # machine-readable

Exit code 1 when any layer lags upstream, or when a layer cannot be read.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.request
from pathlib import Path

UPSTREAM = ("https://raw.githubusercontent.com/AzurLaneTools/"
            "AzurLaneLuaScripts/main/CN/sharecfg/activity_template.lua")

WORK = Path(os.environ.get("BELFAST_WORK_DIR", r"E:\Agent工作区\apkwork"))
MIRROR = WORK / "al-lua" / "CN" / "sharecfg" / "activity_template.lua"
BUILT = WORK / "belfast-data-97" / "CN" / "ShareCfg" / "activity_template.json"

DISTRO = os.environ.get("BELFAST_WSL_DISTRO", "Ubuntu-26.04")
PG_USER = os.environ.get("BELFAST_PG_USER", "belfast")
PG_DB = os.environ.get("BELFAST_PG_DB", "belfast")

MARK_RE = re.compile(r"\bmark = (\d{8})")

# Postgres is a portable install under ~/pg inside WSL: psql is not on PATH and it
# needs the bundled libs, so resolve both here instead of trusting the login shell.
PSQL_BASH = (
    'export LD_LIBRARY_PATH="$HOME/pg/usr/lib/x86_64-linux-gnu"; '
    'PB=$(ls -d "$HOME"/pg/usr/lib/postgresql/*/bin 2>/dev/null | head -1); '
    '[ -n "$PB" ] || { echo "no postgres bin found under ~/pg" >&2; exit 1; }; '
    '"$PB/psql" -h 127.0.0.1 -U ' + PG_USER + ' -d ' + PG_DB + ' -At -c "$1"'
)


def max_mark_in_text(text: str) -> str:
    marks = MARK_RE.findall(text)
    return max(marks) if marks else "-"


def upstream_mark() -> tuple[str, str]:
    try:
        raw = urllib.request.urlopen(UPSTREAM, timeout=120).read()
    except Exception as exc:                                   # noqa: BLE001
        return "-", f"fetch failed: {exc}"
    return max_mark_in_text(raw.decode("utf-8", errors="replace")), ""


def db_mark() -> tuple[str, str]:
    sql = ("SELECT string_agg(DISTINCT (data->>'mark'), ',') FROM config_entries "
           "WHERE category = 'ShareCfg/activity_template.json' AND data ? 'mark'")
    try:
        proc = subprocess.run(
            ["wsl", "-d", DISTRO, "-u", "niu", "-e", "bash", "-c", PSQL_BASH, "--", sql],
            capture_output=True, text=True, encoding="utf-8", timeout=120,
        )
    except Exception as exc:                                   # noqa: BLE001
        return "-", f"psql failed: {exc}"
    if proc.returncode != 0:
        return "-", (proc.stderr.strip().splitlines() or ["psql non-zero exit"])[-1][:200]
    marks = [m for m in (proc.stdout or "").strip().split(",") if m.strip().isdigit()]
    return (max(marks) if marks else "-"), ""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--no-network", action="store_true")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    layers: dict[str, dict[str, str]] = {}

    if args.no_network:
        layers["upstream (lua)"] = {"mark": "-", "note": "skipped (--no-network)"}
    else:
        mark, note = upstream_mark()
        layers["upstream (lua)"] = {"mark": mark, "note": note}

    mirror_text = MIRROR.read_text(encoding="utf-8", errors="replace") if MIRROR.exists() else None
    layers["mirror (al-lua)"] = {
        "mark": max_mark_in_text(mirror_text) if mirror_text else "-",
        "note": "" if mirror_text else f"missing: {MIRROR}",
    }

    if not BUILT.exists():
        layers["built (belfast-data-97)"] = {"mark": "-", "note": f"missing: {BUILT}"}
    else:
        try:
            rows = json.loads(BUILT.read_text(encoding="utf-8", errors="replace"))
            marks = [str(r.get("mark")) for r in rows
                     if isinstance(r, dict) and str(r.get("mark", "")).isdigit()]
            layers["built (belfast-data-97)"] = {"mark": max(marks) if marks else "-", "note": ""}
        except json.JSONDecodeError as exc:
            layers["built (belfast-data-97)"] = {"mark": "-", "note": f"bad json: {exc}"}

    mark, note = db_mark()
    layers["database (config_entries)"] = {"mark": mark, "note": note}

    ref = layers["upstream (lua)"]["mark"]
    comparable = ref != "-"
    stale = [n for n, i in layers.items()
             if n != "upstream (lua)" and comparable and i["mark"] not in (ref, "-")]
    missing = [n for n, i in layers.items() if i["mark"] == "-"]

    if args.json:
        print(json.dumps({"layers": layers, "reference": ref,
                          "stale": stale, "missing": missing}, ensure_ascii=False, indent=2))
        return 1 if stale or missing else 0

    print("activity_template latest `mark` by layer (newest = most current)\n")
    width = max(len(n) for n in layers)
    for name, info in layers.items():
        flag = ""
        if name.startswith("upstream"):
            flag = "  <-- newest upstream"
        elif comparable and info["mark"] not in (ref, "-"):
            flag = "  <-- STALE"
        note = f"   ({info['note']})" if info["note"] else ""
        print(f"  {name:<{width}}  mark={info['mark']}{flag}{note}")

    print()
    if not stale and not missing:
        print("OK: every layer matches upstream.")
    if stale:
        print("STALE: " + ", ".join(stale))
        print("  -> data is behind the live client; private-server validation is not meaningful")
        print("  -> run tools/upgrade-server.ps1 (mirror + rebuild + reseed) first")
    if missing:
        print("MISSING: " + ", ".join(missing) + "  (see notes above)")
    return 1 if stale or missing else 0


if __name__ == "__main__":
    raise SystemExit(main())
