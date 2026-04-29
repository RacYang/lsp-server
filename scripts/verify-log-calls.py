#!/usr/bin/env python3
"""校验日志门面调用的 message、字段命名与日志/指标边界。"""

from __future__ import annotations

import argparse
import codecs
import re
import sys
from pathlib import Path

from lang_verify_common import (
    cjk_ratio,
    collect_by_patterns,
    is_facade_impl,
    load_section,
    posix_rel,
)


def _facade_alias(source: str, facade_pkg: str) -> str:
    esc = re.escape(facade_pkg)
    m = re.search(rf"\bimport\s+(\w+)\s+\"{esc}\"", source)
    if m:
        return m.group(1)
    m = re.search(rf"\bimport\s+\"{esc}\"", source)
    if m:
        return facade_pkg.split("/")[-1]
    blk = re.search(r"import\s*\((.*?)\)", source, re.DOTALL)
    if blk:
        inner = blk.group(1)
        m = re.search(rf"(\w+)\s+\"{esc}\"", inner)
        if m:
            return m.group(1)
        if re.search(rf"\"{esc}\"", inner):
            return facade_pkg.split("/")[-1]
    return ""


def _skip_first_arg(arglist: str) -> str:
    depth = 0
    for i, ch in enumerate(arglist):
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif ch == "," and depth == 0:
            return arglist[i + 1 :].strip()
    return arglist.strip()


def _split_top_args(arglist: str) -> list[str]:
    args: list[str] = []
    start = 0
    depth = 0
    i = 0
    while i < len(arglist):
        ch = arglist[i]
        if ch == '"':
            i = _skip_go_string(arglist, i)
            continue
        if ch == "`":
            end = arglist.find("`", i + 1)
            i = len(arglist) if end == -1 else end + 1
            continue
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif ch == "," and depth == 0:
            args.append(arglist[start:i].strip())
            start = i + 1
        i += 1
    tail = arglist[start:].strip()
    if tail:
        args.append(tail)
    return args


def _skip_go_string(source: str, i: int) -> int:
    i += 1
    escaped = False
    while i < len(source):
        ch = source[i]
        if escaped:
            escaped = False
        elif ch == "\\":
            escaped = True
        elif ch == '"':
            return i + 1
        i += 1
    return i


def _first_go_string(s: str) -> str | None:
    s = s.lstrip()
    if not s:
        return None
    if s.startswith('"'):
        m = re.match(r'"((?:\\.|[^"\\])*)"', s)
        if not m:
            return None
        raw = m.group(1)
        # 仅当存在反斜杠转义时再走 unicode_escape；否则 UTF-8 中文会被误解码为乱码。
        if "\\" not in raw:
            return raw
        try:
            return codecs.decode(raw, "unicode_escape")
        except Exception:
            return raw
    if s.startswith("`"):
        end = s.find("`", 1)
        if end == -1:
            return None
        return s[1:end]
    return None


def _extract_message(arglist: str) -> str | None:
    rest = _skip_first_arg(arglist)
    return _first_go_string(rest)


def _extract_key_literals(arglist: str) -> list[str]:
    args = _split_top_args(arglist)
    # logx.Info(ctx, msg, "key", value, ...)
    keys: list[str] = []
    for idx in range(2, len(args), 2):
        s = _first_go_string(args[idx])
        if s is not None:
            keys.append(s)
    return keys


def _call_iter(source: str, alias: str):
    esc = re.escape(alias)
    pat = re.compile(rf"\b{esc}\.(Info|Debug|Warn|Error)\s*\(", re.MULTILINE)
    for m in pat.finditer(source):
        start = m.end()
        depth = 0
        i = start
        while i < len(source):
            ch = source[i]
            if ch == "(":
                depth += 1
            elif ch == ")":
                if depth == 0:
                    inner = source[start:i]
                    yield inner, source[m.start() : i + 1]
                    break
                depth -= 1
            i += 1


def main() -> int:
    ap = argparse.ArgumentParser(description="校验 logx 调用 message 与 key")
    ap.add_argument("--file", type=Path, help="仅校验单个文件")
    args = ap.parse_args()

    logcfg = load_section("logging")
    facades = list(logcfg["facade_packages"])
    impl_globs = list(logcfg["facade_impl_paths"])
    min_msg = float(logcfg["message_min_cjk_ratio"])
    literal_forbidden = set(logcfg.get("forbidden_literal_keys_in_calls") or [])
    naming = logcfg.get("field_naming") or {}
    key_pattern = re.compile(str(naming.get("pattern") or r"^[a-z][a-z0-9_]*$"))
    max_key_len = int(naming.get("max_length") or 32)
    forbidden_keys = set(logcfg.get("forbidden_field_keys") or [])
    pii_keys = set(logcfg.get("pii_redact_keys") or [])

    com = load_section("commenting")
    paths = com["code_paths"]
    excludes = list(com["code_exclude"])
    facade_pkg = facades[0] if facades else ""

    def check_file(path: Path) -> list[str]:
        rel = posix_rel(path)
        if is_facade_impl(rel, impl_globs):
            return []
        src = path.read_text(encoding="utf-8")
        literal_key_exempt = rel.endswith("_test.go") or "// logx:bridge" in src
        alias = _facade_alias(src, facade_pkg) if facade_pkg else ""
        if not alias:
            return []
        errs: list[str] = []
        for inner, fragment in _call_iter(src, alias):
            msg = _extract_message(inner)
            if msg is None:
                errs.append(f"{path}: 无法解析日志 message，调用: {fragment[:120]!r}")
                continue
            r = cjk_ratio(msg, None)
            if r + 1e-9 < min_msg:
                errs.append(
                    f"{path}: 日志 message 中文占比 {r:.3f} < {min_msg}，message={msg!r}"
                )
            for key in _extract_key_literals(inner):
                if not literal_key_exempt and key in literal_forbidden:
                    errs.append(f"{path}: 日志调用不得手写上下文字段 `{key}`，请通过 Context 注入")
                if len(key) > max_key_len or not key_pattern.match(key):
                    errs.append(f"{path}: 日志字段 `{key}` 不符合命名规则")
                if key in forbidden_keys:
                    errs.append(f"{path}: 日志字段 `{key}` 属于指标候选，请改用 Prometheus 指标")
                if key in pii_keys:
                    errs.append(f"{path}: 日志字段 `{key}` 属于敏感键，请使用脱敏派生字段")
        return errs

    if args.file:
        errs = check_file(args.file.resolve())
        if errs:
            print("\n".join(errs), file=sys.stderr)
            return 1
        return 0

    all_errs: list[str] = []
    for path in collect_by_patterns(paths, excludes):
        all_errs.extend(check_file(path))
    if all_errs:
        print("\n".join(all_errs), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
