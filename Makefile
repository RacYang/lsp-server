ROOT_DIR := $(CURDIR)
GO_BIN := $(shell go env GOPATH)/bin
SHELL := /usr/bin/env PATH=$(GO_BIN):$(PATH) /bin/bash
GENERATED_FILES := .golangci.yml .go-arch-lint.yml .markdownlint.yaml .yamllint.yml .commitlintrc.json
FILE ?=
SCENARIO ?= a
DOCKER ?= docker

HAS_GO := $(shell find . -type f -name '*.go' -not -path './.build/negatives/*' -not -path '*/gen/*' -print -quit 2>/dev/null)
HAS_PROTO := $(shell find api -type f -name '*.proto' -print -quit 2>/dev/null)

.PHONY: bootstrap generate fix fix-file verify verify-fast verify-pre-commit verify-image verify-bench \
	verify-fmt verify-lint verify-arch verify-deps verify-proto verify-proto-break \
	verify-test-fast verify-test verify-test-integration verify-test-integration-nodocker verify-test-integration-pg verify-cover verify-vuln verify-tidy verify-secrets \
	verify-meta verify-config verify-tools verify-determinism verify-commit-msg verify-lang verify-domain verify-redis-keys verify-metrics-naming \
	verify-git-repo verify-git-local verify-git-push

bootstrap:
	@bash scripts/install-tools.sh
	@chmod +x .build/derive.sh scripts/*.sh scripts/*.py .githooks/*
	@mkdir -p .git/hooks
	@ln -sf ../../.githooks/pre-commit .git/hooks/pre-commit
	@ln -sf ../../.githooks/pre-push .git/hooks/pre-push
	@ln -sf ../../.githooks/commit-msg .git/hooks/commit-msg

generate:
	@bash .build/derive.sh "$(ROOT_DIR)"
ifneq ($(HAS_PROTO),)
	@buf generate
endif
	@go mod tidy

fix:
	@files="$$(find . -type f -name '*.go' ! -path '*/gen/*')"; \
	if [[ -n "$$files" ]]; then gofmt -w $$files; goimports -w $$files; fi

fix-file:
	@if [[ -n "$(FILE)" && -f "$(FILE)" && "$(FILE)" == *.go ]]; then gofmt -w "$(FILE)"; goimports -w "$(FILE)"; fi

verify: verify-fmt verify-lint verify-arch verify-deps verify-proto verify-proto-break verify-test verify-test-integration verify-cover verify-vuln verify-tidy verify-secrets verify-meta verify-config verify-tools verify-determinism verify-git-repo verify-lang verify-domain

verify-fast: verify-fmt verify-lint verify-arch verify-deps verify-proto verify-test-fast verify-secrets verify-meta verify-config verify-tools verify-determinism verify-git-repo verify-lang verify-domain

verify-git-repo:
	@python3 scripts/verify-repo-hygiene.py
	@python3 scripts/verify-hooks-parity.py

verify-git-local:
	@python3 scripts/verify-branch-name.py

verify-git-push:
	@bash scripts/verify-git-push.sh

verify-pre-commit: verify-git-local verify-fast

verify-image:
	@$(DOCKER) build -f deploy/docker/gate.Dockerfile -t lsp-gate:local .
	@$(DOCKER) build -f deploy/docker/room.Dockerfile -t lsp-room:local .
	@$(DOCKER) build -f deploy/docker/lobby.Dockerfile -t lsp-lobby:local .

verify-bench:
	@SCENARIO="$(SCENARIO)" bash bench/scripts/run.sh

verify-fmt:
	@files="$$(find . -type f -name '*.go' ! -path '*/gen/*' ! -path './.build/negatives/*')"; \
	if [[ -n "$$files" ]]; then \
		test -z "$$(gofmt -l $$files)" || (echo "gofmt would change files" >&2; gofmt -l $$files; exit 1); \
		test -z "$$(goimports -l $$files)" || (echo "goimports would change files" >&2; goimports -l $$files; exit 1); \
	fi

verify-lint:
ifneq ($(HAS_GO),)
	@golangci-lint run ./...
else
	@echo "verify-lint: no go files, skipping"
endif

verify-arch:
	@go-arch-lint check

verify-deps:
	@bash scripts/dep-guard.sh

verify-proto:
ifneq ($(HAS_PROTO),)
	@buf lint
	@tmp_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	rsync -a --exclude '.git' ./ "$$tmp_dir/repo" >/dev/null; \
	(cd "$$tmp_dir/repo" && buf generate >/dev/null); \
	diff -qr api/gen/go "$$tmp_dir/repo/api/gen/go"
else
	@echo "verify-proto: no proto files, skipping"
endif

verify-proto-break:
ifneq ($(HAS_PROTO),)
	@ref="$$(yq -r '.proto.baseline_ref' .build/config.yaml)"; \
	if git rev-parse --verify "$$ref" >/dev/null 2>&1; then \
		buf breaking --against ".git#ref=$$ref"; \
	else \
		echo "verify-proto-break: baseline not ready, skipping"; \
	fi
else
	@echo "verify-proto-break: no proto files, skipping"
endif

verify-test-fast:
ifneq ($(HAS_GO),)
	@go test ./...
else
	@echo "verify-test-fast: no go packages, skipping"
endif

verify-test:
ifneq ($(HAS_GO),)
	@go test -race -coverprofile=coverage.out ./...
else
	@echo "verify-test: no go packages, skipping"
endif

verify-test-integration:
ifneq ($(HAS_GO),)
	@if [[ "$${RUN_INTEGRATION:-0}" != "1" ]]; then \
		echo "verify-test-integration: RUN_INTEGRATION!=1，跳过"; \
	else \
		go test -tags=integration ./internal/app ./internal/handler ./internal/service/room -run 'Test(ClusterReconnectLoginWithSessionToken|HandleWebSocketIdempotencyKeyDropsReplay|SubmitActionReturnsRateLimitedWhenMailboxFull|SchedulerAutoTimeoutUsesFakeClock)' -count=1 -v; \
	fi
else
	@echo "verify-test-integration: no go packages, skipping"
endif

verify-test-integration-nodocker:
ifneq ($(HAS_GO),)
	@if [[ "$${RUN_INTEGRATION:-0}" != "1" ]]; then \
		echo "verify-test-integration-nodocker: RUN_INTEGRATION!=1，跳过"; \
	else \
		go test -tags=integration ./internal/app ./internal/handler ./internal/service/room -run 'Test(RoomProcessRestartReconnectNoDocker|ClusterReconnectLoginWithSessionToken|HandleWebSocketIdempotencyKeyDropsReplay|SubmitActionReturnsRateLimitedWhenMailboxFull|SchedulerAutoTimeoutUsesFakeClock)' -count=1 -v; \
	fi
else
	@echo "verify-test-integration-nodocker: no go packages, skipping"
endif

verify-test-integration-pg:
ifneq ($(HAS_GO),)
	@if [[ "$${RUN_INTEGRATION:-0}" != "1" ]]; then \
		echo "verify-test-integration-pg: RUN_INTEGRATION!=1，跳过"; \
	else \
		go test -tags=integration ./internal/app -run 'TestRoomProcessRestartReplay' -count=1 -v; \
	fi
else
	@echo "verify-test-integration-pg: no go packages, skipping"
endif

verify-cover:
ifneq ($(HAS_GO),)
	@bash scripts/coverage-gate.sh coverage.out
else
	@echo "verify-cover: no go packages, skipping"
endif

verify-vuln:
ifneq ($(HAS_GO),)
	@govulncheck ./...
else
	@echo "verify-vuln: no go packages, skipping"
endif

verify-tidy:
	@d1="$$( (cat go.mod; test -f go.sum && cat go.sum) | openssl dgst -sha256)"; \
	go mod tidy >/dev/null; \
	d2="$$( (cat go.mod; test -f go.sum && cat go.sum) | openssl dgst -sha256)"; \
	test "$$d1" = "$$d2" || { echo "go.mod 或 go.sum 在 go mod tidy 后发生变化，请检查依赖声明" >&2; exit 1; }

verify-secrets:
	@gitleaks detect --no-banner --no-git --source . --redact

verify-meta:
	@markdownlint-cli2 "docs/**/*.md" "*.md"
	@shellcheck scripts/*.sh .build/derive.sh .githooks/*
	@python3 -m yamllint -c .yamllint.yml .build buf.yaml .github/workflows
	@python3 scripts/verify-meta.py
	@bash scripts/verify-negatives.sh

verify-config:
	@tmp_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	bash .build/derive.sh "$$tmp_dir"; \
	for file in $(GENERATED_FILES); do diff -q "$$file" "$$tmp_dir/$$file"; done

verify-tools:
	@yq -r '.tools | keys[]' .build/config.yaml | while read -r tool; do \
		case "$$tool" in \
			go) \
				expected="$$(yq -r '.tools.go' .build/config.yaml)"; \
				actual="$$(go version | awk '{print $$3}' | sed 's/^go//')"; \
				[[ "$$actual" == "$$expected" ]] || { echo "go version mismatch: $$actual != $$expected" >&2; exit 1; }; \
				;; \
			yamllint) python3 -m yamllint --version >/dev/null 2>&1 || { echo "missing tool: $$tool" >&2; exit 1; } ;; \
			shellcheck) command -v shellcheck >/dev/null 2>&1 || { echo "missing tool: $$tool" >&2; exit 1; } ;; \
			markdownlint-cli2) command -v markdownlint-cli2 >/dev/null 2>&1 || { echo "missing tool: $$tool" >&2; exit 1; } ;; \
			*) command -v "$$tool" >/dev/null 2>&1 || { echo "missing tool: $$tool" >&2; exit 1; } ;; \
		esac; \
	done

verify-determinism:
	@tmp_one="$$(mktemp -d)"; tmp_two="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp_one" "$$tmp_two"' EXIT; \
	bash .build/derive.sh "$$tmp_one"; \
	bash .build/derive.sh "$$tmp_two"; \
	for file in $(GENERATED_FILES); do diff -q "$$tmp_one/$$file" "$$tmp_two/$$file"; done

verify-commit-msg:
	@python3 scripts/verify-commit-msg.py "$(MSG)"

verify-lang:
	@python3 scripts/verify-lang-docs.py
	@python3 scripts/verify-lang-comments.py
	@python3 scripts/verify-no-direct-logging.py
	@python3 scripts/verify-log-calls.py

verify-domain: verify-redis-keys verify-metrics-naming

verify-redis-keys:
	@python3 scripts/verify-redis-keys.py

verify-metrics-naming:
	@python3 scripts/verify-metrics-naming.py
