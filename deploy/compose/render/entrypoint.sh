#!/bin/sh
# 配置渲染入口：将 /templates/*.yaml.template 中的 ${VAR} 占位用环境变量替换后写入 /rendered。
# 仅在 deploy/compose 编排中使用，由一次性容器执行；应用容器不依赖本脚本。
set -eu

if [ ! -d /templates ]; then
  echo "缺少模板挂载点 /templates" >&2
  exit 1
fi

if [ ! -d /rendered ]; then
  echo "缺少输出挂载点 /rendered" >&2
  exit 1
fi

apk add --no-cache gettext >/dev/null

required="POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB"
for name in $required; do
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "缺少必要环境变量: $name" >&2
    exit 1
  fi
done

found=0
for tpl in /templates/*.yaml.template; do
  if [ ! -f "$tpl" ]; then
    continue
  fi
  found=$((found + 1))
  base=$(basename "$tpl")
  out_name=${base%.template}
  out_path="/rendered/$out_name"
  envsubst < "$tpl" > "$out_path"
  chmod 0444 "$out_path"
  echo "渲染完成: $out_path"
done

if [ "$found" -eq 0 ]; then
  echo "/templates 中未找到 *.yaml.template" >&2
  exit 1
fi
