#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${ROOT_DIR}/.build/config.yaml"
GO_BIN="$(go env GOPATH)/bin"
export PATH="${PATH}:${GO_BIN}"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"

bootstrap_install_yq() {
  local version="v4.45.1"
  if command -v yq >/dev/null 2>&1; then
    return
  fi
  go install "github.com/mikefarah/yq/v4@${version}"
}

bootstrap_install_yq

version_of() {
  yq -r ".tools.\"$1\"" "${CONFIG_FILE}"
}

install_go_tool() {
  local tool="$1"
  local package="$2"
  local version
  version="$(version_of "${tool}")"
  go install "${package}@v${version}"
}

install_gitleaks_binary() {
  local version os arch asset tmp_dir
  version="$(version_of gitleaks)"
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "${os}/${arch}" in
    darwin/arm64) asset="gitleaks_${version}_darwin_arm64.tar.gz" ;;
    darwin/x86_64) asset="gitleaks_${version}_darwin_x64.tar.gz" ;;
    linux/x86_64) asset="gitleaks_${version}_linux_x64.tar.gz" ;;
    linux/arm64|linux/aarch64) asset="gitleaks_${version}_linux_arm64.tar.gz" ;;
    *)
      echo "unsupported platform for gitleaks install: ${os}/${arch}" >&2
      exit 1
      ;;
  esac

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  curl -sSfL "https://github.com/gitleaks/gitleaks/releases/download/v${version}/${asset}" -o "${tmp_dir}/gitleaks.tar.gz"
  tar -xzf "${tmp_dir}/gitleaks.tar.gz" -C "${tmp_dir}"
  install "${tmp_dir}/gitleaks" "${GO_BIN}/gitleaks"
}

install_golangci_lint_binary() {
  local version os arch_raw arch asset tmp_dir
  version="$(version_of golangci-lint)"
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch_raw="$(uname -m)"
  case "${arch_raw}" in
    x86_64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "unsupported arch for golangci-lint install: ${arch_raw}" >&2
      exit 1
      ;;
  esac

  asset="golangci-lint-${version}-${os}-${arch}.tar.gz"
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN
  curl -sSfL "https://github.com/golangci/golangci-lint/releases/download/v${version}/${asset}" -o "${tmp_dir}/golangci-lint.tar.gz"
  tar -xzf "${tmp_dir}/golangci-lint.tar.gz" -C "${tmp_dir}"
  if [[ -f "${tmp_dir}/golangci-lint" ]]; then
    install "${tmp_dir}/golangci-lint" "${GO_BIN}/golangci-lint"
  else
    echo "golangci-lint binary not found in archive ${asset}" >&2
    exit 1
  fi
}

if ! command -v golangci-lint >/dev/null 2>&1; then
  install_golangci_lint_binary
fi
install_go_tool "go-arch-lint" "github.com/fe3dback/go-arch-lint"
install_go_tool "buf" "github.com/bufbuild/buf/cmd/buf"
install_go_tool "govulncheck" "golang.org/x/vuln/cmd/govulncheck"
if ! command -v gitleaks >/dev/null 2>&1; then
  install_gitleaks_binary
fi
install_go_tool "goimports" "golang.org/x/tools/cmd/goimports"
install_go_tool "protoc-gen-go" "google.golang.org/protobuf/cmd/protoc-gen-go"
install_go_tool "protoc-gen-go-grpc" "google.golang.org/grpc/cmd/protoc-gen-go-grpc"

if ! command -v markdownlint-cli2 >/dev/null 2>&1; then
  if command -v npm >/dev/null 2>&1; then
    npm install -g "markdownlint-cli2@$(version_of markdownlint-cli2)"
  elif command -v brew >/dev/null 2>&1; then
    brew install node
    npm install -g "markdownlint-cli2@$(version_of markdownlint-cli2)"
  else
    echo "npm is required to install markdownlint-cli2" >&2
    exit 1
  fi
fi

if ! command -v yamllint >/dev/null 2>&1; then
  if command -v pip3 >/dev/null 2>&1; then
    pip3 install "yamllint==$(version_of yamllint)"
  else
    echo "pip3 is required to install yamllint" >&2
    exit 1
  fi
fi

if ! command -v shellcheck >/dev/null 2>&1; then
  if command -v brew >/dev/null 2>&1; then
    brew install shellcheck
  else
    echo "shellcheck is required and could not be auto-installed" >&2
    exit 1
  fi
fi

echo "Tool installation complete."
