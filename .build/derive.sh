#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${ROOT_DIR}/.build/config.yaml"
OUTPUT_DIR="${1:-${ROOT_DIR}}"
HEADER="# GENERATED from .build/config.yaml - DO NOT EDIT"

if ! command -v yq >/dev/null 2>&1; then
  echo "yq is required for derive.sh" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for derive.sh" >&2
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"

CONFIG_FILE="${CONFIG_FILE}" OUTPUT_FILE="${OUTPUT_DIR}/.golangci.yml" \
HEADER="${HEADER}" python3 - <<'PY'
import json
import os
import subprocess

config_file = os.environ["CONFIG_FILE"]
output_file = os.environ["OUTPUT_FILE"]
header = os.environ["HEADER"]

raw = subprocess.check_output(["yq", "-o=json", ".", config_file])
config = json.loads(raw)
generated_marker = config["paths"]["generated_marker"]
golangci = config["lint"]["golangci"]


def scalar(value):
    return json.dumps(str(value), ensure_ascii=False)


def emit_rule(lines, rule, indent):
    lines.append(f"{indent}-")
    nested = indent + "  "
    for key in ("path", "path_except", "linters", "text", "source"):
        if key not in rule:
            continue
        yaml_key = "path-except" if key == "path_except" else key
        value = rule[key]
        if isinstance(value, list):
            lines.append(f"{nested}{yaml_key}:")
            for item in value:
                lines.append(f"{nested}  - {scalar(item)}")
        else:
            lines.append(f"{nested}{yaml_key}: {scalar(value)}")


settings = golangci.get("settings") or {}
legacy_revive = golangci.get("revive") or {}
revive = settings.get("revive") or legacy_revive
gosec = settings.get("gosec") or {}
exclusions = golangci.get("exclusions") or {}
exclude_paths = [f".*/{generated_marker}/.*", *exclusions.get("paths", [])]

lines = [
    header,
    'version: "2"',
    "linters:",
    "  default: none",
    "  enable:",
]
for linter in golangci.get("enable") or []:
    lines.append(f"    - {linter}")

if revive or gosec:
    lines.append("  settings:")
    disable_rules = revive.get("disable_rules") or []
    if disable_rules:
        lines.append("    revive:")
        lines.append("      rules:")
        for rule in disable_rules:
            lines.append(f"        - name: {rule}")
            lines.append("          disabled: true")
    gosec_excludes = gosec.get("excludes") or []
    if gosec_excludes:
        lines.append("    gosec:")
        lines.append("      excludes:")
        for rule in gosec_excludes:
            lines.append(f"        - {rule}")

lines.append("  exclusions:")
lines.append("    paths:")
for path in exclude_paths:
    lines.append(f"      - {scalar(path)}")

rules = exclusions.get("rules") or []
if rules:
    lines.append("    rules:")
    for rule in rules:
        emit_rule(lines, rule, "      ")

lines.extend([
    "formatters:",
    "  enable:",
    "    - gofmt",
    "    - goimports",
    "  exclusions:",
    "    paths:",
])
for path in exclude_paths:
    lines.append(f"      - {scalar(path)}")

lines.extend([
    "run:",
    "  timeout: 5m",
])

with open(output_file, "w") as fh:
    fh.write("\n".join(lines) + "\n")
PY

# go-arch-lint v3: derived from .build/config.yaml lint.arch
CONFIG_FILE="${CONFIG_FILE}" OUTPUT_FILE="${OUTPUT_DIR}/.go-arch-lint.yml" \
HEADER="${HEADER}" python3 - <<'PY'
import os
import json
import subprocess

config_file = os.environ["CONFIG_FILE"]
output_file = os.environ["OUTPUT_FILE"]
header = os.environ["HEADER"]

raw = subprocess.check_output(["yq", "-o=json", ".lint.arch", config_file])
arch = json.loads(raw)
layers = arch.get("layers") or {}
deny_rules = arch.get("deny") or []

layer_names = list(layers.keys())
layer_set = set(layer_names)

for rule in deny_rules:
    src = rule.get("from")
    if src not in layer_set:
        raise SystemExit(f"deny.from references unknown layer: {src}")
    for dst in rule.get("to") or []:
        if dst not in layer_set:
            raise SystemExit(f"deny.to references unknown layer: {dst}")

denied = {name: set() for name in layer_names}
for rule in deny_rules:
    denied[rule["from"]].update(rule.get("to") or [])

lines = [
    header,
    "version: 3",
    "workdir: .",
    "allow:",
    "  depOnAnyVendor: true",
    "  ignoreNotFoundComponents: true",
    "excludeFiles:",
    "  - \"^.*_test\\\\.go$\"",
    "components:",
]
for name in layer_names:
    path = layers[name]
    lines.append(f"  {name}:")
    lines.append(f"    in: {path}")

lines.append("deps:")
for name in layer_names:
    blocked = denied[name]
    if blocked:
        allowed = [n for n in layer_names if n != name and n not in blocked]
        lines.append(f"  {name}:")
        if allowed:
            lines.append("    mayDependOn:")
            for dep in allowed:
                lines.append(f"      - {dep}")
        else:
            lines.append("    anyVendorDeps: true")
    else:
        lines.append(f"  {name}:")
        lines.append("    anyProjectDeps: true")

with open(output_file, "w") as fh:
    fh.write("\n".join(lines) + "\n")
PY

cat >"${OUTPUT_DIR}/.markdownlint.yaml" <<EOF
${HEADER}
default: true
MD013: false
MD025: false
MD033: false
MD041: false
EOF

cat >"${OUTPUT_DIR}/.yamllint.yml" <<EOF
${HEADER}
extends: default
rules:
  line-length:
    max: 120
    level: warning
  document-start: disable
EOF

types_json="$(yq -o=json '.commit.conventional_types' "${CONFIG_FILE}")"
scopes_required="$(yq -r '.commit.scopes_required' "${CONFIG_FILE}")"
scope_pattern_json="$(yq -o=json '.commit.scope_pattern' "${CONFIG_FILE}")"
summary_min_cjk_ratio="$(yq -r '.commit.summary_min_cjk_ratio' "${CONFIG_FILE}")"
summary_disallow_terminal_punctuation="$(yq -r '.commit.summary_disallow_terminal_punctuation' "${CONFIG_FILE}")"
forbidden_trailers_json="$(yq -o=json '(.git.trailers.forbidden // [])' "${CONFIG_FILE}")"
cat >"${OUTPUT_DIR}/.commitlintrc.json" <<EOF
{
  "generated": true,
  "headerPattern": "^([a-z]+)(\\\\(([^)]+)\\\\))?: (.+)$",
  "types": ${types_json},
  "scopesRequired": ${scopes_required},
  "scopePattern": ${scope_pattern_json},
  "summaryMinCjkRatio": ${summary_min_cjk_ratio},
  "summaryDisallowTerminalPunctuation": ${summary_disallow_terminal_punctuation},
  "forbiddenTrailers": ${forbidden_trailers_json}
}
EOF
