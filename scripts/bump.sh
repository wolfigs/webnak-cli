#!/usr/bin/env bash
# Sets the release version everywhere it is written down, so the npm package,
# the Claude Code plugin and the MCP server it pins all agree. Then tag it.
#   scripts/bump.sh 0.1.5 && git commit -am "0.1.5" && git tag -a v0.1.5 -m v0.1.5
set -euo pipefail
cd "$(dirname "$0")/.."
V="${1:?usage: scripts/bump.sh <version>}"
python3 - "$V" <<'PY'
import json, re, sys
v = sys.argv[1]
for p in ["npm/package.json", ".claude-plugin/plugin.json"]:
    d = json.load(open(p)); d["version"] = v
    open(p, "w").write(json.dumps(d, indent=2, ensure_ascii=False) + "\n")
m = json.load(open(".mcp.json"))
m["webnak"]["args"] = ["-y", f"@wolfigs/webnak-cli@{v}", "mcp"]
open(".mcp.json", "w").write(json.dumps(m, indent=2) + "\n")
PY
echo "version $V in npm/package.json, .claude-plugin/plugin.json and .mcp.json"
