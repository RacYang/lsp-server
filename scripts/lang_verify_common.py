#!/usr/bin/env python3
"""语言治理脚本共享工具：从 SSOT 读配置、路径匹配、中文占比。"""

from __future__ import annotations

import fnmatch
import json
import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CONFIG = ROOT / ".build" / "config.yaml"


def load_section(name: str) -> dict:
    raw = subprocess.check_output(
        ["yq", "-o", "json", f".{name}", str(CONFIG)],
        text=True,
    )
    return json.loads(raw or "{}")


def posix_rel(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def excluded(rel: str, patterns: list[str]) -> bool:
    for pat in patterns:
        if fnmatch.fnmatch(rel, pat):
            return True
    return False


def cjk_ratio(text: str, keywords: list[str] | None = None) -> float:
    """中文占比：cjk / (cjk + 拉丁字母)。关键词从统计中移除（替换为空格）。"""
    t = text
    if keywords:
        for kw in sorted(keywords, key=len, reverse=True):
            t = t.replace(kw, " ")
    cjk = sum(1 for ch in t if "\u4e00" <= ch <= "\u9fff")
    latin = sum(1 for ch in t if ch.isascii() and ch.isalpha())
    denom = cjk + latin
    if denom == 0:
        return 1.0
    return cjk / denom


def collect_by_patterns(patterns: list[str], excludes: list[str]) -> list[Path]:
    seen: set[Path] = set()
    out: list[Path] = []
    for pat in patterns:
        for p in _glob_from_root(pat):
            if not p.is_file():
                continue
            rel = posix_rel(p)
            if excluded(rel, excludes):
                continue
            rp = p.resolve()
            if rp in seen:
                continue
            seen.add(rp)
            out.append(p)
    return sorted(out)


def _glob_from_root(pattern: str) -> list[Path]:
    """支持 docs/**/*.md、*.md、.cursor/rules/*.mdc 等形式。"""
    if any(ch in pattern for ch in "*?["):
        return list(ROOT.glob(pattern))
    p = ROOT / pattern
    return [p] if p.is_file() else list(p.parent.glob(p.name)) if p.parent.exists() else []


_URL_RE = re.compile(r"https?://[^\s)>\]]+")
_FENCE_RE = re.compile(r"^```[\s\S]*?^```", re.MULTILINE)
_INLINE_CODE_RE = re.compile(r"`[^`]+`")


def strip_markdown_body(raw: str) -> str:
    """去掉 frontmatter、代码围栏、行内代码、URL，保留正文用于语言统计。"""
    text = raw
    if text.startswith("---\n"):
        end = text.find("\n---\n", 4)
        if end != -1:
            text = text[end + 5 :]
    text = _FENCE_RE.sub(" ", text)
    text = _INLINE_CODE_RE.sub(" ", text)
    text = _URL_RE.sub(" ", text)
    return text


def is_facade_impl(rel: str, patterns: list[str]) -> bool:
    return excluded(rel, patterns)


def extract_go_comment_text(source: str) -> str:
    """从 Go 源码中提取注释文本（不含字符串与 rune）；忽略 //go: 指令与常见生成头。"""
    i = 0
    n = len(source)
    chunks: list[str] = []

    def starts_with(prefix: str, idx: int) -> bool:
        return source.startswith(prefix, idx)

    while i < n:
        ch = source[i]
        if ch in (" \t\r"):
            i += 1
            continue
        if ch == "\n":
            i += 1
            continue

        # 单行注释
        if ch == "/" and i + 1 < n and source[i + 1] == "/":
            j = i + 2
            while j < n and source[j] not in "\n":
                j += 1
            line = source[i + 2 : j].strip()
            if not line.startswith("go:") and not _is_generated_comment_line(line):
                chunks.append(line)
            i = j if j < n and source[j] == "\n" else j
            continue

        # 块注释
        if ch == "/" and i + 1 < n and source[i + 1] == "*":
            j = source.find("*/", i + 2)
            if j == -1:
                break
            block = source[i + 2 : j]
            for ln in block.splitlines():
                ln = ln.strip()
                if ln and not _is_generated_comment_line(ln):
                    chunks.append(ln)
            i = j + 2
            continue

        # 原始字符串 `...`
        if ch == "`":
            j = source.find("`", i + 1)
            if j == -1:
                break
            i = j + 1
            continue

        # 解释字符串 "..."
        if ch == '"':
            if starts_with('""', i):
                i += 2
                continue
            i = _skip_interpreted_string(source, i + 1)
            continue

        # 字符字面量 'x'、'\n'、'字'
        if ch == "'":
            i = _skip_char_lit(source, i + 1)
            continue

        i += 1

    return "\n".join(chunks)


def _skip_char_lit(source: str, i: int) -> int:
    """跳过从首字符到闭合单引号（不含起始引号）。"""
    n = len(source)
    escaped = False
    while i < n:
        c = source[i]
        if escaped:
            escaped = False
            i += 1
            continue
        if c == "\\":
            escaped = True
            i += 1
            continue
        if c == "'":
            return i + 1
        i += 1
    return n


def _is_generated_comment_line(line: str) -> bool:
    low = line.lower()
    return "code generated" in low or "do not edit" in low


def _skip_interpreted_string(source: str, i: int) -> int:
    n = len(source)
    escaped = False
    while i < n:
        c = source[i]
        if escaped:
            escaped = False
            i += 1
            continue
        if c == "\\":
            escaped = True
            i += 1
            continue
        if c == '"':
            return i + 1
        i += 1
    return i
