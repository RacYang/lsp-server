#!/usr/bin/env python3
from __future__ import annotations

import json
import pathlib
import re
import shutil
import subprocess
import sys
from typing import Any


import argparse

ROOT = pathlib.Path(__file__).resolve().parents[1]
CONFIG_FILE = ROOT / ".build" / "config.yaml"
CONFIG_SCHEMA_FILE = ROOT / ".build" / "schema" / "config.schema.json"


def load_yaml_json(path: pathlib.Path) -> Any:
    yq = shutil.which("yq")
    if yq is None:
        gopath = subprocess.run(["go", "env", "GOPATH"], check=True, capture_output=True, text=True).stdout.strip()
        candidate = pathlib.Path(gopath) / "bin" / "yq"
        yq = str(candidate) if candidate.exists() else "yq"
    result = subprocess.run(
        [yq, "-o=json", ".", str(path)],
        check=True,
        capture_output=True,
        text=True,
    )
    return json.loads(result.stdout)


def fail(path: str, message: str) -> None:
    raise ValueError(f"{path}: {message}")


def validate(instance: Any, schema: dict[str, Any], path: str = "$") -> None:
    if "enum" in schema and instance not in schema["enum"]:
        fail(path, f"value {instance!r} is not in enum")

    expected_type = schema.get("type")
    if expected_type:
        type_ok = {
            "object": isinstance(instance, dict),
            "array": isinstance(instance, list),
            "string": isinstance(instance, str),
            "integer": isinstance(instance, int) and not isinstance(instance, bool),
            "number": (isinstance(instance, (int, float)) and not isinstance(instance, bool)),
            "boolean": isinstance(instance, bool),
        }.get(expected_type)
        if not type_ok:
            fail(path, f"expected {expected_type}")

    if expected_type == "object":
        properties = schema.get("properties", {})
        pattern_properties = schema.get("patternProperties", {})
        for key in schema.get("required", []):
            if key not in instance:
                fail(path, f"missing required property {key}")
        for key, value in instance.items():
            if key in properties:
                validate(value, properties[key], f"{path}.{key}")
                continue
            matched = False
            for pattern, child_schema in pattern_properties.items():
                if re.fullmatch(pattern, key):
                    validate(value, child_schema, f"{path}.{key}")
                    matched = True
                    break
            if not matched and schema.get("additionalProperties") is False:
                fail(path, f"unexpected property {key}")

    if expected_type == "array":
        if len(instance) < schema.get("minItems", 0):
            fail(path, "array has too few items")
        item_schema = schema.get("items")
        if item_schema:
            for idx, item in enumerate(instance):
                validate(item, item_schema, f"{path}[{idx}]")
        if schema.get("uniqueItems") and len(instance) != len({json.dumps(v, sort_keys=True) for v in instance}):
            fail(path, "array items must be unique")

    if expected_type == "string":
        if len(instance) < schema.get("minLength", 0):
            fail(path, "string is too short")
        if "pattern" in schema and not re.fullmatch(schema["pattern"], instance):
            fail(path, f"string does not match pattern {schema['pattern']}")

    if expected_type in {"integer", "number"}:
        if "minimum" in schema and instance < schema["minimum"]:
            fail(path, f"value is below minimum {schema['minimum']}")
        if "maximum" in schema and instance > schema["maximum"]:
            fail(path, f"value is above maximum {schema['maximum']}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--file", type=pathlib.Path, help="负例模式：校验指定的 YAML 文件")
    args = ap.parse_args()

    try:
        config = load_yaml_json(args.file if args.file else CONFIG_FILE)
        schema = json.loads(CONFIG_SCHEMA_FILE.read_text())
        validate(config, schema)
    except (ValueError, subprocess.CalledProcessError, json.JSONDecodeError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
