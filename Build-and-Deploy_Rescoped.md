# TL;DR

`Build-and-Deploy.md` describes the intended pipeline. This document records what the repository actually does today, the gap between the two, and the scope decisions taken to close it.

Headline: **coverage already meets the 80% bar (82.9%) — it simply is not being measured.** Three defects will stop `make pipeline` from working, and one of them fails *silently*, which is worse than the two that fail loudly.

`Build-and-Deploy.md` remains the record of the intent.

---

# Summary

## 1. The builds

Everything is driven by `make`. `make help` prints this list from the Makefile itself.

### Local development

| Target | Produces | Depends on | Requires |
|---|---|---|---|
| `fmt` | formatted source | — | Go |
| `vet` | static analysis result | — | Go |
| `build` | `bin/agora`, a single native binary | `vet` | Go 1.22+, **CGO and a C toolchain** (sqlite-vec is a loadable extension) |
| `run` | the server on `$HOST_PORT`, no Docker | `build` | as above |

### Quality gates

| Target | Produces | Depends on | Notes |
|---|---|---|---|
| `test` | test result + `coverage.out` | `build` | Hermetic — no server, no network, no API key. Reports coverage against `COVERAGE_MIN` (default 80); **reports, does not fail** |
| `coverage` | `coverage.html` | `test` | For eyeballing which statements are uncovered |
| `test-gates` | isolation + anonymity gate results | — | Fast feedback on the two claims the submission rests on |
| `cloud-parity` | full suite at `AGORA_K=1` | — | Proves the anonymity descope seam still works |

Current state: **82.6%**, above the 80% target.

### Artifacts

| Target | Produces | Depends on | Requires |
|---|---|---|---|
| `package` | `agora:latest` for the **host architecture** | `test` | Docker. No registry credentials |
| `package-linux` | `agora:latest` for **linux/amd64** | `test` | Docker with buildx. Use this when deploying from Apple Silicon to an x86 host |
| `push` | image on Docker Hub | `package-linux` | `DOCKERHUB_REPO=<user>/agora` and a prior `docker login`. Opt-in |

The image is the only binary artifact. It is self-contained: binary, `seed/`, and curl for the healthcheck.

### End-to-end

| Target | Does | Depends on |
|---|---|---|
| `up` | starts the container, waits for health, prints `/healthz` | `package` |
| `down` | stops it **and deletes the data volume** | — |
| `seed` | re-seeds a **running** instance (`POST /v1/demo/reset`) | — (needs a running container) |
| `pipeline` | `package` → compose up → seed → `demo.sh` → teardown | `package` |
| `pipeline-parity` | the same at `AGORA_K=1`, asserting via `/healthz` that k=1 is genuinely live | `package` |

`pipeline` is the real gate: `demo.sh` asserts on the walkthrough summary and exits non-zero, so a broken system turns it red.

### Deploy

| Target | Does |
|---|---|
| `bootstrap-host DEPLOY_HOST=<ip>` | **installs** Docker Engine + the Compose v2 plugin on the host over SSH and adds the login user to the docker group. Idempotent. Never run automatically — installing packages mutates someone else's machine |
| `preflight-host DEPLOY_HOST=<ip>` | checks the host has Docker, Compose **v2**, a reachable daemon without sudo, and reports its arch — before anything is transferred |
| `deploy-host DEPLOY_HOST=<ip>` | preflight → build linux/amd64 → stream image over SSH → copy compose → start → **both** verification steps below |
| `test-remote-in-image DEPLOY_HOST=<ip>` | runs the bundled client **inside** the remote container, against `localhost:8080` |
| `verify-host DEPLOY_HOST=<ip>` | polls `/healthz`, then runs the client **from your machine** across the network |
| `test-remote` | alias for `verify-host` |
| `test-in-image` | the in-container client against the **local** container |

### Three test surfaces, and why the order matters

The same external client runs at three distances from the code. Each one rules out a different cause, so running them in order turns "it's broken" into a one-line diagnosis.

| Surface | Command | Proves | Rules out |
|---|---|---|---|
| 1. In-process | `make test` | the logic is correct | nothing about packaging or networking |
| 2. In-container | `test-in-image` / `test-remote-in-image` | the **deployed artifact** works — image, seed, store, policy | the network path entirely; `localhost` never leaves the container |
| 3. External | `make pipeline` / `verify-host` | the **network path** works — published port, firewall, security group | — |

The useful case is surface 2 passing while surface 3 fails: the application is fine and the fault is the firewall or the security group. Without the in-container run you cannot tell that from an application bug, and you will spend the afternoon reading logs.

This is why the image carries `demo.sh`, `curl` and `jq` (~1.5 MB): a remote host with no tooling of its own can still verify the artifact it is running.

**Two deliberate deviations from a strict chain**, explained in full under *Target chain* below: `package` depends on `test` rather than `build` (the Dockerfile compiles in its own multi-stage build, so the local binary is not an input), and `seed` has no prerequisite (the container seeds itself on startup).

---

## 2. Deploying on any Ubuntu host (or any SSH-reachable machine)

`DEPLOY_HOST` is just a variable name — these steps work against any reachable Ubuntu box: EC2, a VPS, or a VM on your desk.

### Credentials — the key never enters the repository

The Makefile records the **path** to an SSH key, never the key. Settings come from files that are not committed:

```
Makefile defaults  <  ~/.agora/deploy.env  <  ./deploy.env  <  environment  <  make VAR=…
```

Both env files are optional — `-include` does not error on a missing file — and both are gitignored. `deploy.env.example` is the only one committed, and it contains no real values.

```bash
cp deploy.env.example deploy.env     # then set DEPLOY_KEY to a path outside the repo
make deploy-host DEPLOY_HOST=<ip>        # picks it up automatically, no extra flags
```

Or without a file at all:

```bash
make deploy-host DEPLOY_HOST=<ip> DEPLOY_KEY=~/.ssh/agora.pem
DEPLOY_KEY=~/.ssh/agora.pem make deploy-host DEPLOY_HOST=<ip>
```

Leave `DEPLOY_KEY` empty and everything still works through your ssh-agent or `~/.ssh/config` — `SSH_IDENTITY` collapses to nothing and the Makefile calls plain `ssh`.

**`make check-key` is a guard, not a convenience.** It runs automatically before `preflight-host` and `deploy-host`, and it never reads the key — it only `stat`s it:

| Check | Why |
|---|---|
| file exists | catches a typo before a 40 MB transfer |
| mode is `400` or `600` | ssh silently refuses a world-readable key; the error it prints is not obvious |
| **path is outside the repository** | a key inside the working tree is one `git add .` away from becoming a git object, and git objects are hard to un-publish |

`.gitignore` covers `deploy.env`, `*.pem`, `*.key`, `*.p12` and `id_rsa*` as a second line of defence — but the in-repo check is the one that matters, because a gitignore entry only helps if the file matches the pattern you happened to write down.

### Host distribution — what is and is not distro-specific

**The image is not tied to any host distribution.** It is Debian-based *inside*, which is irrelevant to the host: a container image carries its own userland, so the same artifact runs on Amazon Linux, Ubuntu, RHEL or anything else running Docker. Nothing about packaging changes with the host.

What *is* distro-aware is the host bootstrap, and only there:

| Distro | Docker install | Compose v2 plugin |
|---|---|---|
| Ubuntu / Debian | `get.docker.com` (which also supplies the plugin) | usually already installed by the above |
| **Amazon Linux 2023** | `dnf install docker` | installed separately from the GitHub release |
| **Amazon Linux 2** | `amazon-linux-extras install docker` — AL2 predates `dnf` | installed separately |
| RHEL / CentOS / Rocky / Alma / Fedora | `dnf install docker` | installed separately |

The package manager is chosen by **what is actually present on the box** (`dnf`, then `apt-get`, then `yum`) rather than by mapping distro IDs, because the ID → manager mapping is exactly where Amazon Linux 2 breaks.

Note that no distro but Debian/Ubuntu gets the Compose plugin from its package manager, which is why the bootstrap always version-checks and installs it from the official release when missing or older than 2.17.

`make preflight-host` reports the distro, version and login user before anything is transferred, and warns if `bootstrap-host` does not recognise the distro.

### Prerequisites on the host

Docker Engine plus the Compose **v2 plugin**. Ubuntu's `apt install docker-compose` gives the old Python v1, which does not understand `docker compose` as a subcommand and does not support `--wait`:

```bash
curl -fsSL https://get.docker.com | sh          # engine + compose v2 plugin
sudo usermod -aG docker $USER && newgrp docker  # so docker runs without sudo
docker compose version                           # must be v2.17 or newer for --wait
```

Open the port: an inbound rule for `8080/tcp` in the EC2 security group, or `sudo ufw allow 8080/tcp` on a plain host.

Nothing else is needed on the host. **No Go toolchain, no C compiler, no source checkout** — the image carries everything.

### Path A — push from your laptop (no registry)

The default, and the reason Docker Hub is optional.

```bash
make deploy-host DEPLOY_HOST=<ip>                    # or DEPLOY_USER=… for a non-ubuntu login
```

That builds for linux/amd64, streams the image over SSH (`docker save | gzip | ssh docker load`), copies `docker-compose.yml`, starts the stack, polls `/healthz`, and runs the full external client against it. A deploy that lands broken is reported as broken rather than printing a URL and exiting 0.

Roughly 40 MB crosses the wire per deploy.

### Path B — build on the host

Useful when the host has better bandwidth than your laptop, or when you want to avoid the image transfer entirely.

```bash
git clone <repo> && cd agora
docker compose up -d --wait          # after: make package   (or docker build -t agora:latest .)
```

Still no Go on the host: the Dockerfile's multi-stage build compiles inside the build container.

### Path C — via Docker Hub

Only if you have run `make push`:

```bash
scp docker-compose.yml ubuntu@<ip>:~/
ssh ubuntu@<ip> "IMAGE_NAME=<user>/agora:latest docker compose up -d --wait"
```

### Verify — inside first, then outside

```bash
make test-remote-in-image DEPLOY_HOST=<ip>   # 1. the app, with no network path involved
make verify-host        DEPLOY_HOST=<ip>   # 2. the same client across the internet
```

`deploy-host` runs both in that order automatically. If step 1 passes and step 2 fails, stop reading application logs — the container is healthy and the fault is the published port, the firewall, or the security group.

Or by hand — note that `/healthz` echoes the **live anonymity policy**, so you can confirm which privacy posture is actually running rather than trusting the compose file:

```bash
curl -s http://<ip>:8080/healthz | jq
# {"status":"ok","members":5,"vectors":true,
#  "policy":{"k":2,"gate_justifications":true,"reveal_vetoes":false}, ...}
```

### Operate

```bash
ssh ubuntu@<ip>
docker compose logs -f agora                       # logs
docker compose restart agora                       # restart
curl -sX POST localhost:8080/v1/demo/reset         # re-seed mid-demo
docker compose down                                # stop, KEEP the volume
docker compose down -v                             # stop and wipe the volume
```

Configuration is environment only, read by `docker-compose.yml`:

| Variable | Default | Effect |
|---|---|---|
| `HOST_PORT` | `8080` | published port on the host |
| `AGORA_K` | `2` | anonymity threshold — `1` is cloud parity, anonymity off |
| `IMAGE_NAME` | `agora:latest` | image to run |

To change the privacy posture on a running host:

```bash
AGORA_K=1 docker compose up -d --force-recreate
curl -s http://<ip>:8080/healthz | jq .policy      # confirm it actually took
```

### Five things that will bite

1. **Architecture mismatch.** Building on an Apple Silicon Mac produces an arm64 image; a `t3.*` EC2 instance is x86. It will load and then fail to start with an `exec format error`. Use `make package-linux` (or `deploy-host`, which does it for you).
2. **Compose v1.** `docker-compose` (hyphenated) is the old Python tool and will reject `--wait`. You need `docker compose` v2.17+.
3. **`make down` deletes the data volume.** It passes `-v`, which is right for a pipeline teardown and wrong on a server. Use `docker compose down` there.
4. **State does not survive a restart, by design.** `app.New` calls `Reset` unconditionally on every start, so a restart re-seeds and any conversation a reviewer had is gone. That is deliberate — it keeps the demo deterministic whenever it is opened — but it means the named volume only preserves state *within* a run, and anyone mid-demo should not be restarted.
5. **The port is the firewall's problem, not Docker's.** The container will look perfectly healthy while being unreachable, because `/healthz` passes from inside. Check the security group before debugging the app.

---

# Configuring a fresh clone

What someone who just cloned this repository must change before the targets work. Nothing below is committed with a real value.

## Works with no configuration at all

Every local target runs straight from a clone:

`build` `test` `coverage` `test-gates` `cloud-parity` `package` `package-linux` `up` `down` `seed` `pipeline` `pipeline-parity` `run` `test-in-image`

They need tools, not settings — see *Local prerequisites* below. **`make pipeline` is the one to run first**: it is the full local gate and touches no remote host.

## Required only for deployment

These live in `deploy.env`, which is gitignored and does not exist until you create it:

```bash
cp deploy.env.example deploy.env
```

| Field | Default | Required for | What to set it to |
|---|---|---|---|
| `DEPLOY_HOST` | *(empty)* | `preflight-host`, `deploy-host`, `verify-host`, `test-remote-in-image` | Public IP or DNS of **your** instance. Can also be passed per-command: `make deploy-host DEPLOY_HOST=54.x.x.x` |
| `DEPLOY_KEY` | *(empty)* | the same targets, unless you use an ssh-agent | Absolute path to **your own** `.pem`. Must live **outside the repository** and be mode 400/600 — `make check-key` enforces both. Leave empty to fall back to your ssh-agent or `~/.ssh/config` |
| `DEPLOY_USER` | `ubuntu` | the same targets | Login user for your AMI: `ec2-user` (Amazon Linux), `ubuntu` (Ubuntu), `admin` (Debian). **Wrong value looks like an auth failure, not a config error** |
| `PLATFORM` | `linux/amd64` | `package-linux`, `deploy-host` | `linux/amd64` for t2/t3/t5, `linux/arm64` for Graviton (t4g/c7g/m7g). `preflight-host` warns on a mismatch |
| `HOST_PORT` | `8080` | everything | Published port. Must match the inbound rule in your security group |
| `AGORA_K` | `2` | everything | Anonymity threshold: `2` family default, `1` cloud parity. Verify what took effect with `/healthz` |
| `DOCKERHUB_REPO` | *(empty)* | `push` only | `yourname/agora`, plus a prior `docker login`. Not needed for anything else — the EC2 path streams over SSH and uses no registry |

Precedence, lowest to highest:

```
Makefile defaults  <  ~/.agora/deploy.env  <  ./deploy.env  <  environment  <  make VAR=…
```

So a one-off override never requires editing a file:

```bash
make deploy-host DEPLOY_HOST=54.1.2.3 DEPLOY_USER=ec2-user DEPLOY_KEY=~/.ssh/mine.pem
```

## First-run sequence

```bash
cp deploy.env.example deploy.env     # 1. create your local settings
$EDITOR deploy.env                   # 2. set DEPLOY_HOST, DEPLOY_USER, DEPLOY_KEY
chmod 600 ~/.ssh/your-key.pem        # 3. ssh refuses a loose key
make check-key                       # 4. validates path, mode, and not-in-repo
make bootstrap-host DEPLOY_HOST=<ip>     # 5. installs Docker + Compose v2 on the host
make preflight-host DEPLOY_HOST=<ip>     # 6. re-verifies (bootstrap already does this)
make deploy-host    DEPLOY_HOST=<ip>     # 7. deploy, then both verification steps
```

Step 5 is only needed on a host that has never been bootstrapped; it is safe to
re-run and skips whatever is already correct. A bare Ubuntu AMI needs it. A host
you have deployed to before does not.

Steps 4 and 5 exist so a misconfiguration costs seconds rather than a completed 40 MB image transfer that then fails.

## What you must set up on the AWS side

None of this lives in the repo, and `deploy.env` cannot substitute for any of it:

1. **A key pair**, with the `.pem` downloaded and `chmod 600`. This is *your* key — the repository contains no key and never will.
2. **An inbound rule for `HOST_PORT`** (8080 by default) in the instance's security group, from your IP. Get this wrong and the container will look perfectly healthy while being unreachable, because `/healthz` passes from inside the container.
3. ~~**Docker Engine plus the Compose v2 plugin** on the instance~~ — **handled for you by `make bootstrap-host`**, since the SSH access needed to deploy is the same access needed to install. It detects the distro, installs the engine, installs the Compose v2 plugin if missing or older than 2.17 (the version that added `--wait`), creates the docker group if absent, and adds your login user to it. Ubuntu's `apt install docker-compose` would install the old Python v1, which rejects `--wait` — the bootstrap deliberately avoids it.
4. **An instance whose architecture matches `PLATFORM`.** `linux/amd64` (the default) suits t2/t3/t5; Graviton needs `PLATFORM=linux/arm64`. `preflight-host` compares the host's `uname -m` against `PLATFORM` and prints the exact fix, because the symptom otherwise is `exec format error`, which reads like a corrupt transfer rather than a platform mismatch.

`make preflight-host` checks items 3 and 4 and, where the fix is mechanical, points at `make bootstrap-host`. It cannot check item 2, because from outside a closed port and an unreachable host look identical.

**What still needs you, and why bootstrap cannot help:** the key in item 1 is yours and must never be in the repository; the inbound rule in item 2 is an AWS control-plane change, not something reachable over SSH to the instance. Everything else on this list is mechanical and therefore automated.

## Local prerequisites

| Tool | Needed for | Note |
|---|---|---|
| Go 1.22+ **with CGO and a C toolchain** | `build`, `test`, `run` | sqlite-vec is a CGO extension. macOS: Xcode command line tools. Debian/Ubuntu: `build-essential` and `libsqlite3-dev` |
| Docker + Compose v2 | `package`, `up`, `pipeline`, `deploy-host` | `docker compose version` must be 2.17+ for `--wait` |
| `jq` | the assertion step in `demo.sh` | Without it the script exits 2 rather than silently passing |
| `bc` | the coverage report in `test` | Present by default on macOS and most Linux |

Note that the **image** needs none of this — it carries the binary, seed data, curl and jq. A host only needs Docker.

---

# Verified facts

Measured against the repository, not inferred.

| Claim | Status | Evidence |
|---|---|---|
| Unit tests reach ≥80% coverage | **True — 82.9%** | `go test ./test/... -coverpkg=./internal/...` |
| `make test` measures that coverage | **False — reports 0%** | Tests live in the external `test` package; `go test ./...` counts only same-package coverage |
| The compose healthcheck can pass | **False** | `curl` is not installed in `debian:bookworm-slim`; the image adds only `ca-certificates` |
| `AGORA_K` configures the policy | **False — silently ignored** | `main.go` env-maps `AGORA_ADDR`, `AGORA_DB`, `AGORA_SEED`; `k` is flag-only |
| `make pipeline` fails when the system is broken | **False** | `demo.sh` prints the summary and exits 0 regardless |
| `package` pushes to Docker Hub | **False** | `docker build` only |
| Each target builds on the previous | **Partly** | Only `pipeline: package` is wired |

---

# Blocking defects

## B1 — The healthcheck can never pass, so `make pipeline` never starts

`docker-compose.yml` health-checks with `curl -f http://localhost:8080/v1/members`. The runtime stage installs only `ca-certificates`, so there is no `curl` (and no `wget`) in the image. Every probe fails, the container never reports healthy, `docker compose up -d --wait` exhausts its retries, and `pipeline` dies before running a single test.

**Decision: give the binary a self-check rather than adding a shell tool to the image.**

- `GET /healthz` — a cheap liveness endpoint that touches the store.
- `agora --healthcheck` — dials `/healthz` on the configured address and exits 0/1.
- Compose healthcheck becomes `CMD ["/app/agora", "--healthcheck"]`.

This keeps the runtime image minimal and, more usefully, means the health contract is *compiled and testable* rather than a string in a YAML file that nothing verifies. The alternative — `apt-get install curl` — is more conventional and easier to poke at with `docker exec`, and is noted as the fallback if debuggability inside the container matters more than image surface.

## B2 — `AGORA_K` is silently ignored

Compose sets `AGORA_K=${AGORA_K:-2}`. `main.go` never reads it. A deployment intending cloud parity (`k=1`) runs the family policy instead and looks fine.

This one matters more than it first appears: `k` is the **anonymity threshold**. A silently-wrong `k` is a silently-wrong privacy posture, and the whole submission rests on that being correct and inspectable.

**Decision: two fixes, not one.**

1. `k` gets an env fallback like every other setting: `flag.Int("k", envOrInt("AGORA_K", 2), ...)`.
2. The policy is **echoed on the health endpoint and at startup**, so a reviewer can confirm which policy is live rather than trusting the deployment. A privacy setting that cannot be observed from outside is a privacy setting nobody can audit.

## B3 — `demo.sh` cannot fail, so the pipeline cannot fail

`Build-and-Deploy.md` defines pipeline testing as "a client that will test various scenarios using `curl` like an external user". Today that client prints results and always exits 0.

**Decision: `demo.sh` asserts and exits non-zero.** It already calls `/v1/demo/walkthrough`, which returns `summary.criteria_met` and `summary.of` — the script compares them and fails the build on mismatch. A pipeline that cannot go red is decoration.

Secondary: `demo.sh` degrades to `cat` when `jq` is absent, which makes the assertion unparseable. The script will require `jq` for the assertion step and say so plainly rather than silently skipping it.

---

# Target chain

`Build-and-Deploy.md` says "each target builds on a previous target." Wiring that literally is wrong in one place, so the chain is made explicit where the dependency is real and deliberately absent where it is not.

```
fmt ─ vet ─┐
           ├─▶ build ──▶ test ──▶ package ──▶ pipeline
           │                         │
           │                         ├─▶ push        (opt-in)
           │                         └─▶ deploy-host
           └─────────────────────────────▶ seed      (needs a RUNNING instance)
```

Two deliberate deviations from a strict chain:

**`package` does not depend on `build`.** The Dockerfile compiles in its own multi-stage build, so a local binary is not an input to the image. Making `package` depend on `build` would compile the code twice and, worse, imply the image contains the locally-built artifact when it does not. `package` depends on `test` instead, which is the dependency that actually protects something.

**`seed` cannot depend on `package`.** `Build-and-Deploy.md` describes `seed` as "builds the package and loads it with mock-data", but the container **seeds itself on startup** — `app.New` calls `Reset` unconditionally, so a fresh container is already loaded with mock data. `make seed` is therefore a *re-seed* against a running instance, which is genuinely useful mid-demo but is not a build step. Renamed in spirit, kept as `seed`, documented as requiring a running instance.

---

# Test

Two modes, as specified.

## Mode 1 — unit tests, ≥80% coverage

Already satisfied at **82.9%**, but unmeasured. Changes:

- `make test` gains `-coverpkg=./internal/...` so external-package tests are counted.
- A `COVERAGE_MIN` threshold (default 80) is enforced by the target, failing the build below it.
- `make coverage` writes `coverage.html` for inspection.

One caveat worth stating rather than burying: with `-coverpkg`, the number measures *statements exercised by the suite*, not the traditional per-package unit-test coverage. Given that the suite is deliberately black-box — it drives the system through the same surfaces a reviewer does — that is the more honest figure, but it is not the same metric a Go developer expects from `go test -cover`, and the doc should not pretend otherwise.

## Mode 2 — pipeline, external client over curl

`make pipeline` already has the right shape: `package` → `compose up --wait` → seed → external client → teardown via `trap`. With B1 and B3 fixed it becomes a real gate. Additions:

- The run asserts on `summary.criteria_met == summary.of` and exits non-zero otherwise.
- A `pipeline-parity` variant runs the same external client against a container started with `AGORA_K=1`, which — once B2 is fixed — verifies the anonymity descope seam **through the deployed artifact**, not just in the unit suite. The seam is now checked at both levels.

---

# Deploy

`docker compose up -d` on an EC2 host, image streamed over SSH. That is sound, and notably it **needs no registry at all**, which is why the Docker Hub push below is optional rather than load-bearing.

Fixes:

- **`PORT` vs `HOST_PORT`.** The Makefile uses `PORT`, compose uses `HOST_PORT`. `make deploy-host PORT=9000` would map 8080 and print the wrong URL. Unified on `HOST_PORT`, exported to compose.
- **`deploy-host` does not verify.** It ends by printing a URL. It should poll `/healthz` and then run the external client remotely, so a deploy that lands broken is reported as broken.
- `docker save | gzip | ssh docker load` moves ~40MB per deploy. Fine at this cadence; a registry is the alternative if it becomes tedious.

---

# Code review

The final pass in `Build-and-Deploy.md`: review each non-generated source file and trim what is not required.

Proposed order, cheapest signal first:

1. **`internal/arbiter/anonymity.go`** — the invariant is that nothing outside this file references `k` or the public/private classification. That is mechanically checkable with a grep, and it is what keeps the descope a one-line change.
2. **`internal/agent/internal/rawctx/`** and the `arbiter.Store` interface — the two halves of the compiler-enforced boundary.
3. **`internal/arbiter/reconcile.go`** — the only privacy-critical code path that is not a model call.
4. Everything else, for dead code and unused exports.

Worth adding as a review artifact: a test that asserts the `anonymity.go` invariant, so the boundary is enforced by CI rather than by remembering.

---

# Open decisions

| # | Decision | Recommendation |
|---|---|---|
| D1 | Docker Hub push | Separate opt-in `push` target, not part of `package`. The EC2 path streams over SSH and needs no registry, so folding a credentialed push into `package` makes the common path fail without secrets configured |
| D2 | Coverage gate | Hard-fail at 80% via `COVERAGE_MIN`. Already passing at 82.9%, so the gate costs nothing today and catches the regression later |
| D3 | Healthcheck mechanism | `agora --healthcheck` self-check, keeping the runtime image minimal |
