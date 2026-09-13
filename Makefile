SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# Deployment settings live OUTSIDE the repository.
#
# Both files are optional (`-include` never errors on a missing file) and both
# are gitignored. Precedence, lowest to highest:
#   defaults below  <  ~/.agora/deploy.env  <  ./deploy.env  <  environment  <  make VAR=…
#
# See deploy.env.example for the format. Put the key path there, not the key.
-include $(HOME)/.agora/deploy.env
-include deploy.env

BIN          := bin/agora
IMAGE_NAME   ?= agora:latest
# Deployment is not AWS-specific: any reachable host with SSH will do.
DEPLOY_HOST  ?=
DEPLOY_USER  ?= ubuntu
# Path to the SSH private key (.pem). Leave empty to use your ssh-agent or
# ~/.ssh/config instead. The key itself must never live in this repository —
# `make check-key` refuses if it does.
DEPLOY_KEY   ?=
# Target platform for the deployable image. t2/t3/t5 instances are x86_64;
# Graviton (t4g/c7g/m7g) is arm64. preflight-host warns when this does not match
# the host it is about to deploy to.
PLATFORM     ?= linux/amd64
HOST_PORT    ?= 8080
AGORA_K      ?= 2
# Provider for containerised runs. The binary walks a chain — local model, then
# ANTHROPIC_API_KEY, then the deterministic extractor — and these targets decide
# what the container is given.
#
# LOCAL TARGETS HAND IT NEITHER. No model, no key, so the chain lands on the
# simulated extractor: nothing to download, nothing to bill, seconds not
# minutes. That is a property of the targets, not a flag you must remember, and
# it holds even if your shell happens to export ANTHROPIC_API_KEY.
#
# The DEPLOYED host is where the chain earns its keep: it has a key and no
# Ollama, so the same image lands on Anthropic without being told to.
#   make pipeline                            -> simulated (deterministic)
#   make pipeline AGORA_LLM_PREFER=ollama    -> local Ollama, needs `make ollama-setup`
#   make pipeline AGORA_LLM_PREFER=anthropic -> Anthropic, BILLED
AGORA_LLM_PREFER ?=
# Optional. Empty means "ask the provider what it has" (see EnsureModel).
AGORA_LLM_MODEL  ?=
# Optional override for the OpenAI-compatible endpoint (Groq, OpenRouter, a
# second local runtime). Setting it selects that provider on its own.
AGORA_LLM_BASE   ?=
# Model pulled by `make ollama-setup`. 3B is the smallest size that reliably
# returns parseable JSON for the tier-1 extraction prompts.
OLLAMA_MODEL     ?= llama3.2:3b
OLLAMA_LOG       ?= $(HOME)/.agora/ollama.log
COVERAGE_MIN ?= 80
COVER_OUT    := coverage.out

# HOST_PORT and AGORA_K are read by docker-compose.yml, so exporting them here
# means one knob drives the Makefile, compose and the deployed container.
# Previously the Makefile used PORT while compose used HOST_PORT, so changing
# the port moved the printed URL without moving the published port.
export HOST_PORT
export AGORA_K
export IMAGE_NAME
export AGORA_LLM_PREFER
export AGORA_LLM_MODEL
export AGORA_LLM_BASE

# sqlite-vec is a loadable extension, so CGO is required for local builds.
export CGO_ENABLED = 1

# Secrets file, sourced by the SHELL inside recipes — deliberately NOT a make
# variable and NOT -include'd. A make variable can surface in `make -n` output,
# in argv, and in error messages; a shell-sourced file cannot.
SECRETS_FILE ?= $(HOME)/.agora/secrets.env
# Loads ANTHROPIC_API_KEY (and optionally AGORA_LLM_MODEL) if the file exists.
LOAD_SECRETS  = set -a; [ -f "$(SECRETS_FILE)" ] && . "$(SECRETS_FILE)"; set +a;

# Build the ssh/scp invocations once. When DEPLOY_KEY is empty these collapse to
# plain ssh/scp, so an ssh-agent or ~/.ssh/config setup keeps working unchanged.
# accept-new trusts a first-seen host key but still fails on a CHANGED one,
# which is what you want for a freshly launched instance without disabling
# host verification outright.
ifneq ($(strip $(DEPLOY_KEY)),)
  SSH_IDENTITY := -i $(DEPLOY_KEY) -o IdentitiesOnly=yes
endif
SSH_OPTS := -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10
SSH      := ssh $(SSH_IDENTITY) $(SSH_OPTS)
SCP      := scp $(SSH_IDENTITY) $(SSH_OPTS)

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[-a-zA-Z0-9_]+:.*?##/ && $$2 !~ /@internal/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## --- Build & Test ---

.PHONY: fmt
fmt: ## @internal Format Go source code
	go fmt ./...

.PHONY: vet
vet: ## @internal Run go vet analysis
	go vet ./...

.PHONY: build
build: vet ## Build local binary: gofmt check + vet + compile (requires CGO)
	@# A CHECK, not `go fmt`. A build target that rewrites your source files is
	@# surprising mid-edit and makes the working tree depend on build order.
	@# `make fmt` is the writer; this only refuses to build unformatted code.
	@unformatted=$$(gofmt -l cmd internal test 2>/dev/null); \
	 if [ -n "$$unformatted" ]; then \
	   echo "unformatted files (run: make fmt):"; echo "$$unformatted" | sed 's/^/  /'; exit 1; fi
	@mkdir -p bin
	go build -o $(BIN) ./cmd/agora

## test measures coverage with -coverpkg because the suite lives in an external
## `test` package: plain `go test ./...` counts only same-package coverage and
## would report 0% for everything in internal/. The threshold is REPORTED, not
## enforced — see COVERAGE_MIN.
.PHONY: test
test: build ## Run unit tests with coverage (hermetic; no running server needed)
	@# Scoped to ./test/... because that is where the whole suite lives. Running
	@# ./... here would additionally print a misleading "0.0% of statements" line
	@# for every internal package — they have no in-package tests by design.
	@# Compilation of every package is already checked by the `build` prerequisite.
	go test ./test/... -count=1 -coverpkg=./internal/... -coverprofile=$(COVER_OUT)
	@echo
	@total=$$(go tool cover -func=$(COVER_OUT) | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}'); \
	 printf "coverage: %s%% (target %s%%)\n" "$$total" "$(COVERAGE_MIN)"; \
	 if (( $$(echo "$$total < $(COVERAGE_MIN)" | bc -l) )); then \
	   printf "\033[33mWARNING: coverage %s%% is below the %s%% target\033[0m\n" "$$total" "$(COVERAGE_MIN)"; \
	 else \
	   printf "\033[32mcoverage target met\033[0m\n"; \
	 fi

.PHONY: test-gates
test-gates: ## Run property gates only (IsolationGate and AnonymityGate)
	go test ./test/... -count=1 -run 'IsolationGate|AnonymityGate' -v

## --- Packaging & Container ---

## package depends on test, NOT on build: the Dockerfile compiles in its own
## multi-stage build, so the local binary is not an input to the image. Making
## it depend on build would compile twice and imply the image contains the
## locally built artifact. Depending on test is the dependency that protects
## something.
.PHONY: package
package: test ## Build the image. Cross-build with: make package PLATFORM=linux/amd64
	@if [ -n "$(strip $(PLATFORM))" ]; then \
	  echo "building for $(PLATFORM)"; docker build --platform $(PLATFORM) -t $(IMAGE_NAME) .; \
	else docker build -t $(IMAGE_NAME) .; fi

## push is deliberately separate and opt-in. Folding it into `package` would
## make the common path — and therefore `pipeline` — fail for anyone without
## Docker Hub credentials, including a reviewer who just cloned the repo.
## The deploy path streams the image over SSH and needs no registry at all.

## --- Pipeline & End-to-End ---

## COMPOSE_UP starts the container with the provider that was ASKED FOR, and
## refuses to start one that cannot work.
##
## The refusal is the point. A completion that fails descends a tier by design,
## so a misconfigured provider does not crash — it produces a full, green,
## entirely meaningless run in which every answer came from the deterministic
## extractor. Both failure modes below were observed in exactly that way:
## Ollama bound to 127.0.0.1 (unreachable from a container) and an empty model
## name (rejected with "model is required").
define COMPOSE_UP
	case "$(strip $(AGORA_LLM_PREFER))" in \
	  anthropic) \
	    $(LOAD_SECRETS) \
	    if [ -z "$${ANTHROPIC_API_KEY:-}" ]; then \
	      echo "  FAIL: AGORA_LLM_PREFER=anthropic but no ANTHROPIC_API_KEY."; \
	      echo "        See: make check-secret"; exit 1; fi; \
	    echo "  provider: anthropic (BILLED)"; \
	    docker compose up -d --wait ;; \
	  ollama|local) \
	    if ! curl -sf --max-time 2 http://localhost:11434/v1/models >/dev/null 2>&1; then \
	      echo "  FAIL: AGORA_LLM_PREFER=$(AGORA_LLM_PREFER) but nothing is answering"; \
	      echo "        on http://localhost:11434.  Fix: make ollama-setup"; exit 1; fi; \
	    echo "  provider: local Ollama via host.docker.internal (no tokens spent)"; \
	    AGORA_LLM_BASE="$${AGORA_LLM_BASE:-http://host.docker.internal:11434/v1}" \
	      docker compose up -d --wait ;; \
	  "") \
	    echo "  provider: deterministic (simulated) — no model and no key given"; \
	    echo "            to the container, so the chain ends here. Free and fast."; \
	    ANTHROPIC_API_KEY= AGORA_LLM_BASE= AGORA_LLM_PREFER= docker compose up -d --wait ;; \
	  simulated|deterministic|none|off) \
	    echo "  provider: deterministic (simulated), by request"; \
	    ANTHROPIC_API_KEY= AGORA_LLM_BASE= AGORA_LLM_PREFER=simulated docker compose up -d --wait ;; \
	  *) \
	    echo "  FAIL: AGORA_LLM_PREFER=$(AGORA_LLM_PREFER) is not one of:"; \
	    echo "        ollama, anthropic, simulated (or empty for the default)."; exit 1 ;; \
	esac
endef

## ANTHROPIC_API_KEY= in the default branch is deliberate and load-bearing. The
## binary's chain reaches for a key whenever it finds one, so a developer whose
## shell exports ANTHROPIC_API_KEY would otherwise have `make pipeline` quietly
## start billing them. The local default must be free on every machine, not just
## on ones with a clean environment.

## CHECK_PROVIDER asserts that the container is running the provider that was
## asked for, from OUTSIDE the process. /healthz reports what was CONFIGURED,
## which is necessary but not sufficient — for a local model the container must
## also be able to reach the host, so that path is exercised for real.
define CHECK_PROVIDER
	want="$(strip $(AGORA_LLM_PREFER))"; [ -n "$$want" ] || want=deterministic; \
	case "$$want" in local) want=ollama ;; esac; \
	got=$$(curl -sf http://localhost:$(HOST_PORT)/healthz | tr -d ' \n' \
	       | sed -n 's/.*"provider":"\([a-z-]*\)".*/\1/p'); \
	if [ "$$got" != "$$want" ]; then \
	  echo "  FAIL: asked for $$want, container is running $$got."; \
	  echo "        A run that silently used a different provider would prove"; \
	  echo "        nothing about the path you were testing."; exit 1; fi; \
	echo "  provider $$got, verified via /healthz"; \
	if [ "$$want" = "ollama" ]; then \
	  docker exec agora curl -sf --max-time 5 \
	    http://host.docker.internal:11434/v1/models >/dev/null 2>&1 \
	    && echo "  container can reach the model host, verified from inside" \
	    || { echo "  FAIL: the container cannot reach Ollama at"; \
	         echo "        host.docker.internal:11434. It is almost certainly bound"; \
	         echo "        to 127.0.0.1, which a container cannot route to."; \
	         echo "        Fix: make ollama-down && make ollama-setup"; exit 1; }; fi
endef

.PHONY: up
up: package ## Start the container via docker compose and wait for health
	@$(COMPOSE_UP)
	@echo "agora is up on http://localhost:$(HOST_PORT)"
	@curl -sf http://localhost:$(HOST_PORT)/healthz | sed 's/^/  /' || true

.PHONY: down
down: ## Stop the container and remove its volume
	docker compose down -v

## seed re-seeds a RUNNING instance. It cannot depend on `package`, because the
## container seeds itself on startup: app.New calls Reset unconditionally, so a
## fresh container already holds the mock data. This target is for resetting
## mid-demo.

## pipeline is a real gate: demo.sh asserts on the walkthrough summary and
## exits non-zero, so a broken system turns this red.
.PHONY: pipeline
pipeline: package ## Full pipeline: start container, seed, run external curl client, teardown
	@echo "Starting container via docker compose (k=$(AGORA_K), port $(HOST_PORT))..."
	@# A container's localhost is the CONTAINER, so a model running on this host
	@# is only reachable at host.docker.internal. Detected per-run rather than at
	@# parse time, so targets that do not need it pay nothing.
	@$(COMPOSE_UP)
	@trap 'echo "Tearing down..."; docker compose down -v' EXIT; \
		echo "Confirming the container is running the policy we asked for (k=$(AGORA_K))..." && \
		curl -sf http://localhost:$(HOST_PORT)/healthz | grep -q '"k": $(AGORA_K)' \
		  && echo "  live policy k=$(AGORA_K), verified via /healthz" \
		  || { echo "  FAIL: container is not running k=$(AGORA_K)"; exit 1; } && \
		echo "Confirming the provider is the one we asked for..." && \
		{ $(CHECK_PROVIDER) } && \
		curl -sf -X POST http://localhost:$(HOST_PORT)/v1/demo/reset >/dev/null && \
		echo "Running external client walkthrough (demo.sh)..." && \
		bash ./demo.sh http://localhost:$(HOST_PORT)

## pipeline-parity exercises the anonymity descope seam through the DEPLOYED
## artifact, not just the unit suite. It only proves anything because AGORA_K
## is now actually read by the binary.

## The image bundles demo.sh, curl and jq, so the external client can run from
## INSIDE the container. That isolates the application from the network path:
## if this passes and `verify-host` fails, the fault is the firewall or the
## security group, not the app.
.PHONY: test-in-image
test-in-image: ## Run the bundled client inside the container. Remote: make test-in-image DEPLOY_HOST=<ip>
	@if [ -n "$(strip $(DEPLOY_HOST))" ]; then \
	  echo "in-image client on $(DEPLOY_HOST) (localhost inside the container)"; \
	  $(SSH) $(DEPLOY_USER)@$(DEPLOY_HOST) "docker exec agora bash /app/demo.sh http://localhost:8080"; \
	else docker exec agora bash /app/demo.sh http://localhost:8080; fi

.PHONY: run
run: build ## Run locally without Docker on $(HOST_PORT) (simulated provider)
	@mkdir -p data
	@# Secrets are loaded ONLY when the paid path is asked for by name. Sourcing
	@# them unconditionally would put a key in the environment, and the chain
	@# would then pick Anthropic for anyone who has ever run make set-remote-secret
	@# — turning `make run` into a billed command on a developer's own box.
	@if [ "$(strip $(AGORA_LLM_PREFER))" = "anthropic" ]; then \
	  $(LOAD_SECRETS) \
	  if [ -z "$${ANTHROPIC_API_KEY:-}" ]; then \
	    echo "FAIL: AGORA_LLM_PREFER=anthropic but no ANTHROPIC_API_KEY. See: make check-secret"; exit 1; fi; \
	  echo "provider: anthropic (BILLED)"; \
	  ./$(BIN) --db ./data/agora.db --seed ./seed --addr :$(HOST_PORT) --k $(AGORA_K); \
	else \
	  ANTHROPIC_API_KEY= ./$(BIN) --db ./data/agora.db --seed ./seed --addr :$(HOST_PORT) --k $(AGORA_K); \
	fi

## --- Local model (developer box only) ---

## Ollama is a DEVELOPER-BOX concern, never a deploy-time one: the deployed
## host runs Anthropic or the simulated extractor. It therefore lives in its own
## targets that nothing else depends on, so `make pipeline` and `make deploy-host`
## never pull a 2GB model on someone's behalf.
##
## OLLAMA_HOST=0.0.0.0 is the load-bearing detail. Ollama binds 127.0.0.1 by
## default, and a container cannot route to the host's loopback — so the default
## bind produces a container that reports provider=ollama and silently answers
## every request from the deterministic extractor. NOTE: this listens on every
## interface, so on an untrusted network bind it to the docker bridge instead.
.PHONY: ollama-setup
ollama-setup: ## Install Ollama + pull $(OLLAMA_MODEL), reachable from containers
	@if ! command -v ollama >/dev/null 2>&1; then \
	  if ! command -v brew >/dev/null 2>&1; then \
	    echo "FAIL: ollama is not installed and neither is Homebrew."; \
	    echo "      Install it from https://ollama.com/download, then re-run this."; exit 1; fi; \
	  echo "Installing ollama via Homebrew..."; brew install ollama; \
	@# NOT `ollama --version`: with no server running it prints "could not
	@# connect to a running Ollama instance", which reads like a failure here.
	else echo "  ollama: installed at $$(command -v ollama)"; fi
	@if curl -sf --max-time 2 http://localhost:11434/v1/models >/dev/null 2>&1; then \
	  if lsof -nP -iTCP:11434 -sTCP:LISTEN 2>/dev/null | grep -q '127\.0\.0\.1:11434'; then \
	    echo "FAIL: a server is running but bound to 127.0.0.1, which no container"; \
	    echo "      can reach. Restart it: make ollama-down && make ollama-setup"; exit 1; fi; \
	  echo "  server: already running and reachable from containers"; \
	else \
	  echo "Starting ollama on 0.0.0.0:11434 (log: $(OLLAMA_LOG))..."; \
	  mkdir -p $(dir $(OLLAMA_LOG)); \
	  OLLAMA_HOST=0.0.0.0:11434 nohup ollama serve >> $(OLLAMA_LOG) 2>&1 & \
	  for i in $$(seq 1 20); do \
	    curl -sf --max-time 1 http://localhost:11434/v1/models >/dev/null 2>&1 && break; sleep 1; done; \
	  curl -sf --max-time 2 http://localhost:11434/v1/models >/dev/null 2>&1 \
	    || { echo "FAIL: server did not come up. See $(OLLAMA_LOG)"; exit 1; }; \
	  echo "  server: started"; fi
	@if ollama list 2>/dev/null | grep -q '^$(OLLAMA_MODEL)[[:space:]]'; then \
	  echo "  model:  $(OLLAMA_MODEL) already present"; \
	else echo "Pulling $(OLLAMA_MODEL) (a few GB, one time)..."; ollama pull $(OLLAMA_MODEL); fi
	@echo
	@echo "Ready. Run the pipeline against it with:"
	@echo "  make pipeline AGORA_LLM_PREFER=ollama"

.PHONY: ollama-down
ollama-down: ## Stop the local Ollama server (leaves the downloaded model in place)
	@pkill -f "ollama serve" 2>/dev/null && echo "  stopped" || echo "  not running"

## A convenience alias, so the local-model run is one word rather than a flag
## nobody remembers. It is the ONLY place a default model name is applied: the
## binary otherwise asks the provider what it has.
.PHONY: pipeline-local
pipeline-local: ## Full pipeline against the local Ollama (requires: make ollama-setup)
	@$(MAKE) --no-print-directory pipeline AGORA_LLM_PREFER=ollama AGORA_LLM_MODEL=$(OLLAMA_MODEL)

## --- Configuration checks ---

.PHONY: check
check: check-key check-secret ## Validate deploy key and API key configuration

## --- Secrets ---

.PHONY: check-secret
check-secret: ## @internal Report whether an Anthropic API key is configured (never prints it)
	@$(LOAD_SECRETS) \
	 if [ -z "$${ANTHROPIC_API_KEY:-}" ]; then \
	   echo "ANTHROPIC_API_KEY: NOT SET — the app runs the deterministic extractor."; \
	   echo "  tier 1 will be rule-based, not a model."; \
	   echo "  Set it:  mkdir -p $(dir $(SECRETS_FILE)) && chmod 700 $(dir $(SECRETS_FILE))"; \
	   echo "           printf 'ANTHROPIC_API_KEY=sk-ant-...\\n' > $(SECRETS_FILE)"; \
	   echo "           chmod 600 $(SECRETS_FILE)"; \
	 else \
	   echo "ANTHROPIC_API_KEY: set ($${#ANTHROPIC_API_KEY} chars, ends $${ANTHROPIC_API_KEY: -4})"; \
	   echo "  model: $${AGORA_LLM_MODEL:-claude-sonnet-4-5 (default)}"; \
	 fi
	@if git ls-files --error-unmatch $(notdir $(SECRETS_FILE)) >/dev/null 2>&1; then \
	   echo "FAIL: a secrets file is TRACKED BY GIT. Remove it from the index now."; exit 1; fi
	@# Requires real key SHAPE, not the literal placeholder: an earlier version
	@# matched 'sk-ant-api' and flagged this Makefile's own example text, which is
	@# how a security check earns a reputation for crying wolf and gets ignored.
	@hits=$$(grep -rIlE --exclude-dir=.git 'sk-ant-[A-Za-z0-9_-]{30,}' . 2>/dev/null || true); \
	 if [ -n "$$hits" ]; then \
	   echo "FAIL: API key material is present in the working tree:"; \
	   echo "$$hits" | sed 's/^/  /'; exit 1; \
	 else echo "  no API key material in the working tree"; fi

.PHONY: list-models
list-models: ## Ask the API which models this key can use (avoids guessing an identifier)
	@$(LOAD_SECRETS) \
	 if [ -z "$${ANTHROPIC_API_KEY:-}" ]; then echo "ANTHROPIC_API_KEY is not set"; exit 1; fi; \
	 curl -s https://api.anthropic.com/v1/models \
	   -H "x-api-key: $$ANTHROPIC_API_KEY" -H "anthropic-version: 2023-06-01" \
	 | jq -r '.data[]? | "  \(.id)\t\(.display_name // "")"' || echo "  request failed"

## Sends the key to the host over STDIN, never as a command-line argument:
## argv is world-readable via `ps` on the remote box.
.PHONY: set-remote-secret
set-remote-secret: check-key ## Install the API key on the host (stdin only, 600, never in argv)
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required."; exit 1; fi
	@$(LOAD_SECRETS) \
	 if [ -z "$${ANTHROPIC_API_KEY:-}" ]; then echo "ANTHROPIC_API_KEY is not set locally"; exit 1; fi; \
	 printf 'ANTHROPIC_API_KEY=%s\nAGORA_LLM_MODEL=%s\n' "$$ANTHROPIC_API_KEY" "$${AGORA_LLM_MODEL:-}" \
	 | $(SSH) $(DEPLOY_USER)@$(DEPLOY_HOST) \
	     'umask 077; cat > ~/agora.env && chmod 600 ~/agora.env && echo "  wrote ~/agora.env (mode 600)"'

## --- Deployment (any SSH-reachable host: EC2, a VPS, a VM on your desk) ---

## check-key validates the SSH key WITHOUT ever reading or printing it, and
## refuses a key stored inside the repository — the one mistake that turns a
## private key into a git object.
.PHONY: check-key
check-key: ## @internal Validate DEPLOY_KEY (path, permissions, and that it is not in the repo)
	@if [ -z "$(strip $(DEPLOY_KEY))" ]; then \
	  echo "DEPLOY_KEY is not set — using your ssh-agent / ~/.ssh/config."; \
	  echo "  To use a .pem:  make deploy-host DEPLOY_HOST=<ip> DEPLOY_KEY=~/.ssh/agora.pem"; \
	  echo "  Or persist it:  cp deploy.env.example deploy.env   (gitignored)"; \
	  exit 0; \
	fi; \
	key="$$(eval echo $(DEPLOY_KEY))"; \
	if [ ! -f "$$key" ]; then echo "FAIL: DEPLOY_KEY not found at $$key"; exit 1; fi; \
	repo="$$(cd . && pwd -P)"; keydir="$$(cd "$$(dirname "$$key")" && pwd -P)"; \
	case "$$keydir/" in "$$repo"/*) \
	  echo "FAIL: the key lives inside the repository ($$key)."; \
	  echo "      Move it out — e.g. mv \"$$key\" ~/.ssh/ — so it cannot be committed."; \
	  exit 1;; esac; \
	perms="$$(stat -c '%a' "$$key" 2>/dev/null || stat -f '%Lp' "$$key" 2>/dev/null)"; \
	case "$$perms" in 400|600) ;; *) \
	  echo "FAIL: $$key has mode $$perms; ssh requires 400 or 600."; \
	  echo "      Fix with: chmod 600 \"$$key\""; \
	  exit 1;; esac; \
	echo "  key:     $$key (mode $$perms, outside the repo)"

## bootstrap-host installs what the host needs, over the SSH access we already
## have. It is deliberately NOT a prerequisite of deploy-host: installing
## packages mutates someone else's machine, so it stays an explicit step you
## choose to run. It is idempotent and safe to re-run.
##
## It cannot help with the two things that genuinely need you: your own SSH key,
## and an inbound rule for $(HOST_PORT) in the security group.
.PHONY: bootstrap-host
bootstrap-host: check-key ## Install Docker + Compose v2 on the host (Usage: make bootstrap-host DEPLOY_HOST=<ip-or-dns>)
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required."; exit 1; fi
	@echo "Bootstrapping $(DEPLOY_USER)@$(DEPLOY_HOST)..."
	@$(SSH) $(DEPLOY_USER)@$(DEPLOY_HOST) 'bash -s' < scripts/bootstrap-host.sh
	@echo
	@echo "Re-checking over a fresh connection (docker group membership needs a new login)..."
	@$(MAKE) --no-print-directory preflight-host DEPLOY_HOST=$(DEPLOY_HOST)

## Fail fast on the host's prerequisites. Without this the usual failure mode is
## a 40MB image transfer that completes and THEN dies on `docker: command not
## found` or on Compose v1 rejecting --wait.
.PHONY: preflight-host
preflight-host: check-key ## Check the remote host is ready (Usage: make preflight-host DEPLOY_HOST=<ip-or-dns>)
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required."; exit 1; fi
	@echo "Checking $(DEPLOY_USER)@$(DEPLOY_HOST)..."
	@$(SSH) $(DEPLOY_USER)@$(DEPLOY_HOST) '\
	  set -e; \
	  . /etc/os-release; \
	  echo "  distro:  $${PRETTY_NAME:-$$ID}  (id=$$ID$${VERSION_ID:+, version=$$VERSION_ID}$${VERSION_CODENAME:+, codename=$$VERSION_CODENAME})"; \
	  echo "  user:    $$(id -un)"; \
	  case "$$ID" in ubuntu|debian|amzn|rhel|centos|rocky|almalinux|fedora) ;; \
	    *) echo "  WARN: make bootstrap-host does not know $$ID — install Docker + Compose v2 by hand";; esac; \
	  command -v docker >/dev/null || { echo "  FAIL: docker is not installed. Fix: make bootstrap-host DEPLOY_HOST=$(DEPLOY_HOST)"; exit 1; }; \
	  echo "  docker:  $$(docker --version)"; \
	  docker compose version >/dev/null 2>&1 || { echo "  FAIL: Compose v2 plugin missing or too old (v1 rejects --wait). Fix: make bootstrap-host DEPLOY_HOST=$(DEPLOY_HOST)"; exit 1; }; \
	  echo "  compose: v$$(docker compose version --short)"; \
	  docker ps >/dev/null 2>&1 || { echo "  FAIL: cannot reach the docker daemon as this user. Fix: make bootstrap-host DEPLOY_HOST=$(DEPLOY_HOST)"; exit 1; }; \
	  echo "  daemon:  reachable without sudo"; \
	  a=$$(uname -m); \
	  case "$$a" in \
	    x86_64)        w=linux/amd64 ;; \
	    aarch64|arm64) w=linux/arm64 ;; \
	    *)             w=unknown ;; \
	  esac; \
	  if [ "$$w" = "$(PLATFORM)" ]; then echo "  arch:    $$a — matches PLATFORM=$(PLATFORM)"; \
	  else echo "  WARN: host is $$a but the image is built for $(PLATFORM)."; \
	       echo "        It will load and then fail with exec format error."; \
	       echo "        Fix: make deploy-host DEPLOY_HOST=$(DEPLOY_HOST) PLATFORM=$$w"; fi'
	@echo "  preflight OK"

.PHONY: deploy-host
deploy-host: preflight-host package-linux ## Deploy to any SSH-reachable host and verify (Usage: make deploy-host DEPLOY_HOST=<ip-or-dns>)
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required. Run: make deploy-host DEPLOY_HOST=<ip>"; exit 1; fi
	@echo "Streaming image to $(DEPLOY_USER)@$(DEPLOY_HOST)..."
	docker save $(IMAGE_NAME) | gzip | $(SSH) -C $(DEPLOY_USER)@$(DEPLOY_HOST) "docker load"
	@echo "Copying docker-compose.yml..."
	$(SCP) docker-compose.yml $(DEPLOY_USER)@$(DEPLOY_HOST):~/docker-compose.yml
	@echo "Starting container on remote host..."
	@# ~/agora.env carries the key (written by make set-remote-secret). The host
	@# ships no Ollama, so rung 1 of the chain misses in ~400ms and the key
	@# selects Anthropic. With no key the host runs the simulated extractor
	@# rather than failing to start — degraded, not down.
	$(SSH) $(DEPLOY_USER)@$(DEPLOY_HOST) \
	  'set -a; [ -f ~/agora.env ] && . ~/agora.env; set +a; \
	   HOST_PORT=$(HOST_PORT) AGORA_K=$(AGORA_K) docker compose up -d --wait'
	@echo
	@echo "=== step 1: in-image client (proves the app, ignores the network path) ==="
	@$(MAKE) test-in-image DEPLOY_HOST=$(DEPLOY_HOST)
	@echo
	@echo "=== step 2: external client from this machine (proves the network path) ==="
	@$(MAKE) verify-host DEPLOY_HOST=$(DEPLOY_HOST)

## A deploy that ends by printing a URL has not verified anything. This polls
## health and then runs the same external client the local pipeline runs.
.PHONY: verify-host
verify-host: ## Poll health then run the external client from here (Usage: make verify-host DEPLOY_HOST=<ip-or-dns>)
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required."; exit 1; fi
	@echo "Waiting for http://$(DEPLOY_HOST):$(HOST_PORT)/healthz (up to 40s)..."
	@# --connect-timeout is essential, not tidiness: a security group that DROPS
	@# rather than REJECTS leaves the TCP connect hanging until the OS gives up,
	@# roughly two minutes per attempt. Without it this loop appears to freeze on
	@# its first try instead of polling.
	@ok=0; \
	for i in $$(seq 1 20); do \
	  if curl -sf --connect-timeout 2 --max-time 4 "http://$(DEPLOY_HOST):$(HOST_PORT)/healthz" >/dev/null 2>&1; then ok=1; break; fi; \
	  printf '.'; sleep 2; \
	done; echo; \
	if [ $$ok -eq 0 ]; then \
	  echo "  NOT REACHABLE after ~40s — running diagnostics"; echo; \
	  $(MAKE) --no-print-directory diagnose-host; \
	  exit 1; \
	fi
	@curl -sf --max-time 5 "http://$(DEPLOY_HOST):$(HOST_PORT)/healthz" | sed 's/^/  /'
	bash ./demo.sh http://$(DEPLOY_HOST):$(HOST_PORT)
	@echo "Agora is running at http://$(DEPLOY_HOST):$(HOST_PORT)"

## Three concentric checks. Each boundary that works narrows the cause, so the
## answer is which ring fails first rather than a wall of output:
##   inside the container  -> the application
##   the host's loopback   -> port publishing / compose
##   from this machine     -> security group, NACL, or host firewall
.PHONY: diagnose-host
diagnose-host: ## Diagnose why $(DEPLOY_HOST):$(HOST_PORT) is unreachable
	@if [ -z "$(DEPLOY_HOST)" ]; then echo "Error: DEPLOY_HOST is required."; exit 1; fi
	@echo "=== diagnosing http://$(DEPLOY_HOST):$(HOST_PORT) ==="
	@echo
	@echo "1. your public IP (what a security group source rule must allow)"
	@echo "   $$(curl -s --max-time 5 https://checkip.amazonaws.com 2>/dev/null || echo unknown)"
	@echo
	@echo "2. reachability of $(HOST_PORT) from this machine"
	@# curl rather than a raw TCP probe on purpose. An earlier version used
	@# `timeout`, which stock macOS does not ship, so BOTH probes failed and
	@# reported every port closed — including port 22, while the SSH checks
	@# below were succeeding over that very port. curl is present everywhere and
	@# its exit code separates the two failure modes, which IS the diagnosis:
	@#   28 = timed out   -> packets DROPPED (security group / NACL)
	@#    7 = refused     -> host reached, nothing accepted (listener/publishing)
	@rc=0; out=$$(curl -s --connect-timeout 3 --max-time 6 "http://$(DEPLOY_HOST):$(HOST_PORT)/healthz" 2>/dev/null) || rc=$$?; \
	 case $$rc in \
	   0)  echo "   REACHABLE — $$out" ;; \
	   28) echo "   TIMED OUT — packets are being DROPPED before they reach the host."; \
	       echo "               That is a security group / NACL symptom, not an app one." ;; \
	   7)  echo "   CONNECTION REFUSED — the host was reached but nothing accepted."; \
	       echo "               Port publishing or the listener, not the firewall." ;; \
	   6)  echo "   DNS failure for $(DEPLOY_HOST)" ;; \
	   *)  echo "   curl exit $$rc" ;; \
	 esac
	@echo
	@echo "   (SSH reachability is proven by steps 3-8 below running at all —"
	@echo "    no separate probe, because a probe that can lie is worse than none)"
	@$(SSH) $(DEPLOY_USER)@$(DEPLOY_HOST) 'bash -s' -- $(HOST_PORT) < scripts/diagnose-remote.sh \
	  || echo "   ssh failed — could not run the host-side checks"
	@echo
	@echo "=== how to read this ==="
	@echo "  4 FAIL                  -> the application. Check: docker compose logs agora"
	@echo "  4 OK, 5 FAIL            -> port publishing. Re-run make deploy-host"
	@echo "  4 OK, 5 OK, 2 TIMED OUT -> the network path, NOT the app. In order of likelihood:"
	@echo "                             * the inbound rule is on a security group the instance"
	@echo "                               does not actually use — compare step 8 against the SG"
	@echo "                               you edited. This is the common one."
	@echo "                             * rule exists but for the wrong protocol or port range"
	@echo "                             * source is a stale 'My IP' that no longer matches step 1"
	@echo "                             * subnet network ACL denies $(HOST_PORT) inbound or outbound"

## --- Clean ---

## Deliberately NOT part of `clean`, and deliberately not a prerequisite of
## anything. `clean` removes what this repository built; this removes what it
## asked the internet for — several GB of model weights and a Homebrew package
## that other projects on this machine may also be using. Destroying either as a
## side effect of "clean the build" would be a genuinely unwelcome surprise.
.PHONY: clean-external
clean-external: ## Remove externally sourced artifacts: Ollama, its models, the image
	@echo "Removing externally sourced artifacts:"
	@$(MAKE) --no-print-directory ollama-down
	@if command -v ollama >/dev/null 2>&1; then \
	  echo "  removing downloaded models..."; \
	  ollama list 2>/dev/null | awk 'NR>1 {print $$1}' | while read -r m; do \
	    [ -n "$$m" ] && ollama rm "$$m" >/dev/null 2>&1 && echo "    removed $$m"; done; \
	  rm -rf $(HOME)/.ollama/models 2>/dev/null || true; \
	  if command -v brew >/dev/null 2>&1 && brew list ollama >/dev/null 2>&1; then \
	    echo "  uninstalling ollama (Homebrew)..."; brew uninstall ollama >/dev/null && echo "    done"; fi; \
	else echo "  ollama: not installed"; fi
	@rm -f $(OLLAMA_LOG)
	@-docker compose down -v 2>/dev/null
	@-docker rmi $(IMAGE_NAME) 2>/dev/null
	@echo "Done. Run 'make ollama-setup' to reinstall everything above."

.PHONY: clean
clean: ## Clean local binaries, data, coverage and docker artifacts (not $(OLLAMA_MODEL) — see clean-external)
	rm -rf bin data $(COVER_OUT) coverage.html
	-docker compose down -v 2>/dev/null
	-docker rmi $(IMAGE_NAME) 2>/dev/null
