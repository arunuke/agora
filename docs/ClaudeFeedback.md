# Claude Feedback Log

Historical record of collaboration with Claude on Agora: where we started, what changed, and why.

*A note on filenames.* Two things moved after this log was written. The four
`*_Rescoped.md` documents were consolidated into a single
[06_Rescoped.md](06_Rescoped.md), one section each; and the documents were given
`NN_` prefixes so the folder reads in order. Entries below keep the names they
were written with — a record that renames itself is no longer a record.

---

## Sessions and time spent

| # | When | What | Time |
|---|---|---|---|
| 1 | 2026-09-05 → 09-07 | Initial documents, written by hand before any collaboration | *accounted separately* |
| 2 | 2026-09-07 evening → 09-08 morning | Requirements validation, scope reduction, theme anchoring, rescoped design | ≥ 1 h 10 min |
| 3 | 2026-09-12 afternoon | Refinement — removing growth, not adding features | ≥ 40 min |
| 4 | 2026-09-12 evening → 09-13 | Deployment, provider chain, scenario gates, hardening | **3 h 25 min** |

**How these were derived, and what they are worth.** Session 4 is measured: a
conversation log of 1,297 timestamped entries covering 17:17 → 20:42 local,
280 turns. Trimming the eight idle stretches over five minutes — the longest 24
minutes — gives 2 h 48 min of engaged time; the 3 h 25 min figure keeps them,
which is the honest number for elapsed effort.

Sessions 2 and 3 have **no conversation log on this machine**, so their figures
are floors recovered from git commits and file modification times, not
durations. A file's timestamp records its last save and says nothing about the
thinking before it. Both are certainly undercounts — session 2's floor covers a
commit of 45 files including the entire Go implementation.

So: **5 h 15 min is provable across sessions 2–4.** For the work to have
exceeded an eight-hour window, the unlogged portions of sessions 2 and 3 would
have to add more than 2 h 45 min beyond what their timestamps already show.

---

# Session 1 — 2026-09-05 → 09-07 — The initial documents

*Time accounted separately by Arun — written by hand, before any collaboration.*

Requirements, Design, Implementation, Build-and-Deploy, Guidelines, Rationale,
FAQ and ClaudeDirections were written first and committed before Claude saw the
project. They are the specification everything after this was measured against,
and they are preserved unchanged in this folder — the rescoping in
[06_Rescoped.md](06_Rescoped.md) is recorded as a delta against them rather than as
an edit to them.

That order is why this log can be read as a record at all: there is an original
to compare to.

---

# Session 2 — 2026-09-07/08 — Requirements through rescoped design

*≥ 1 h 10 min (floor: commits at 21:21 and 06:31, file writes 21:39 and
05:41–06:29). No conversation log survives, so the real figure is higher.*

The four rounds below ran as one working block. The topic sections that follow
them — Requirements, Implementation, Design and Build & Deploy Tradeoffs —
record decisions taken during it.

## Round 1 — Requirements validation

### Task

Validate `Requirements.md` against the conditions in `Guidelines.md` (the PlatformSWE take-home assignment), per `ClaudeDirections.md`.

### Findings

**Blocking — assignment conformance**

| # | Gap | Consequence |
|---|-----|-------------|
| 1 | No assignment theme declared | Reviewers cannot grade against a stated bar; the rationale doc is required to explain theme choice |
| 2 | Self-contained evaluation not a requirement | Assignment names this *critical*. Demo data appeared only as a collector fallback in `ClaudeDirections.md` |
| 3 | Three deliverables absent from requirements | Deployed prototype, GitHub repo, written + ~5min video rationale, AI transcripts. Video and transcripts had no owner |
| 4 | Scope exceeded budget by ~an order of magnitude | 3 Go services + protobuf/gRPC + SQLite-vec + scraper + Docker + K8s + EKS/GKE + 80% coverage, against a 1–2h target / 8h hard limit. Assignment explicitly grades scoping ability |

**Substantive — product quality**

| # | Gap | Consequence |
|---|-----|-------------|
| 5 | The differentiating idea — cross-agent information isolation — was mentioned once in `Design.md` and never in requirements | Without it, Agora reads as a movie-recommender chatbot |
| 6 | Assumption 2 ("security not in scope") waived away that same property | Direct contradiction with the thesis |
| 7 | Peer group undefined | US3 depends on a data model that requirements never specified |
| 8 | US2 and US4 (periodic recommendations, notifications) need time to pass | The hardest capability would be invisible in a 5-minute review |
| 9 | No acceptance criteria, no non-goals, no success criteria | `README.md` advertised non-goals and a solution approach that did not exist |
| 10 | Collector scraped TMDB/JustWatch live | API keys, rate limits, third-party availability in the reviewer's critical path — conflicts with finding 2 |

### Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Theme | **Theme 3 — Systems & Reliability** | Best fit for a multi-agent arbiter with long-running workflows. Obligates a real failure/concurrency story, which retires the "all communication is synchronous, no failure handling" stance in `Design.md` |
| Build budget | **2–4 hours** | Forces the cut: single deployable unit, in-process module boundaries, SQLite only, no K8s, no live scraper. gRPC and Kubernetes become documented production steps |
| Client surface | **Minimal web page + API** | A multi-agent system is hard to appreciate through curl alone. ~30–45 min of cost to make the coordination legible in seconds; curl still works underneath |
| Headline idea | **Both isolation and workflow, isolation as the demo hook** | See resolution below |

### Key resolution — isolation and reliability are one mechanism

Theme 3 plus "both isolation and workflow" is over budget if built as two features. Resolved by unifying them:

> A convene workflow fans out to member agents in parallel. Each agent returns only a **sealed, derived signal** — never raw preference text. Because the arbiter never needs raw context, it can proceed correctly on partial responses.

One build produces the privacy demo, the partial-failure story, and the long-running workflow. Isolation *buys* graceful degradation rather than trading against it. This became the stated thesis in `Requirements.md`.

### Changes made to Requirements.md

- Added **Theme and Thesis** section naming Theme 3 and the four systems properties to demonstrate.
- Added **Why the analogy holds** — justifies the cloud-infra → movie-night substitution by naming the preserved property.
- Rewrote **Goals** from 4 to 6, promoting private per-user context and reviewer-legible demo to first-class goals.
- Added **Non-Goals** (7 items).
- Split Assumption 2: authentication out of scope, cross-user information isolation in scope.
- Rewrote **User Stories** as 6 stories with Given/When/Then acceptance criteria, adding US4 (isolation, with an automated leak-check assertion), US5 (degradation) and US6 (time-shifted workflow).
- Added **Self-Contained Evaluation and Demo Mode** with seed state and `reset` / `tick` / `chaos` controls.
- Added **Deliverables**, **Success Criteria** and **Open Risks**.

### Open items carried forward to Design

- Group membership model (symmetric? multi-group?) — US3 needs it, requirements deliberately left it to Design.
- What exactly a "sealed signal" contains, and how the arbiter reconciles signals without pooling raw context.
- Deterministic fallback scoring function for the LLM-outage path.
- Checkpoint granularity for workflow durability.

---

## Round 2 — Scope reduction

### Arun's proposals

1. De-scope the collector; ship mock/seeded catalog data as a component instead.
2. Run everything in a single container.
3. Drop REST in favour of gRPC only, with reviewers using `grpcurl`.
4. Fix the prototype to a single family group, removing creation/join semantics.
5. Lead with data isolation and anonymity alongside reliability as the core values.

### Assessment

**Agreed: 1, 2, 4, 5.**

On (1), the reasoning was sound and worth recording: catalog data is world-state, not user-generated context, so removing the collector costs the isolation thesis nothing. Added a condition — retain the normalized schema as an artifact so the production collector has a defined target, keeping "collector is a production item" a real claim rather than a hand-wave.

On (4), noted that this is not the scope-reduction it appears to be from a privacy standpoint. Cross-group isolation is the easy case; within-group anonymity is the hard one, and a small family is its adversarial setting. The cut removes join semantics without softening the claim.

**Pushed back on (3), and Arun agreed to the reverse cut.**

gRPC-only removes fewer components than it appears to:
- Browsers cannot speak gRPC natively, so a web page needs grpc-web plus an Envoy proxy — a component added, not removed. This would have cost the web surface agreed in Round 1.
- It puts a `grpcurl` install, server reflection, and service/method discovery on the reviewer's path, against an assignment that cannot accept submissions requiring local installation.
- `Implementation.md` had already reasoned this out: *"Using grpcurl could have unified a singular interface... but not considered since it adds additional complexity to the client."*

Since agent and arbiter share a binary, there is no wire between them and gRPC would be ceremony around a function call. **Decision: drop gRPC entirely from the prototype**, keep HTTP/JSON on one port serving both the static page and the API, and write the `.proto` as an unwired design artifact.

### Follow-up decision — the package boundary

Arun asked whether two Go packages exporting public methods was sufficient. Refined to: **the arbiter owns an interface that the agent package satisfies** (consumer-defined, per Go convention). Justified by three concrete payoffs rather than abstraction for its own sake:

- The chaos switch is a decorator implementing that interface, so fault injection requires no conditionals inside the workflow.
- The production gRPC split becomes a second implementation of an existing contract, making "the split is mechanical" true rather than aspirational.
- The convene fan-out is genuinely concurrent — goroutine per member under a real `context` deadline — so partial failure is actual cancellation, not a sleep and a fabricated error. Under Theme 3 that distinction is what is being graded.

Same pattern applied to the LLM: one interface, a real provider implementation, and a deterministic local implementation that the degradation path uses. This also satisfies the swappable-provider requirement from `ClaudeDirections.md`.

### Anonymity strength

Chosen: **k-threshold on justifications.** A constraint appears in a group justification only if at least *k* members share it (k=2 for ~5 members). Below threshold it still influences ranking, silently. Arun's framing: *"it shares only information the other user is sharing without leaking all of their context."*

Recorded tradeoff: this makes explanations coarser — "the family leans short" rather than "Priya wants something short." Accepted, mitigated by low *k* and by preserving ranking influence, and flagged for explicit mention in the written rationale rather than glossed over.

### Changes made

- `Requirements.md`: separated isolation from anonymity in the thesis and named the small-group adversarial case; rewrote Goals (7, was 6) around anonymised reconciliation and a seeded catalog; expanded Non-Goals to 9, adding the collector, gRPC/protobuf, group join semantics and multi-group membership; added Assumption 5 (one fixed family); rewrote US4 with separate isolation and anonymity criteria and two build-gate assertions; updated seed state to one family with at least one singleton preference — the case the threshold must suppress; added the explanation-quality tradeoff to Open Risks.
- `FAQ.md`: added six entries covering the transport decision, what keeps the boundary real without gRPC, why removing the collector is safe, why one group is harder rather than easier, the k-threshold, and the explanation-quality tradeoff.

### Net component count

Before: 3 services, protobuf/gRPC, SQLite + sqlite-vec, scraper, Docker images, K8s manifests, managed cluster.
After: 1 binary, 2 packages, 2 interfaces, SQLite, seeded JSON catalog, 1 container, 1 port.

---

## Round 3 — Anchoring the theme on workflow pain

### Arun's direction

Theme 3 asks for a solution to *a real workflow pain point*. Isolation and anonymity should be named explicitly as pain points in a real workflow that the solution addresses — not presented as abstract design properties.

### Assessment

Correct, and the previous framing was measurably weaker. Isolation and anonymity read as elegant architecture rather than as responses to something that hurts. The theme rewards pain; the requirements were describing taste.

Forcing the pain-point framing also surfaced the strongest argument in the project, which had been left implicit: **attribution suppresses the honest input the system runs on.** Anonymity is not a privacy feature layered onto the design — it is the precondition for obtaining data at all. That reframing moves anonymity upstream of correctness, which is what earns it a place in a reliability submission rather than beside one.

### Three pain points named

| Pain | Today | Agora |
|---|---|---|
| 1. Data needed for a correct answer cannot be pooled | Storage on-call *cannot* read OVN Central — team boundary, compliance, blast radius. Diagnosis becomes a serialized human relay in Slack; MTTR is dominated by cross-boundary round-trips, not root-cause difficulty | Arbiter reconciles sealed derived signals; nothing pooled, no boundary crossed, round-trips parallel at machine speed |
| 2. Attribution suppresses sharing | Output names fault ("Networking had the wrong IP binding"), so teams get careful about what they expose — worsening Pain 1. The dynamic blameless postmortems exist to counter | Constraint enters the explanation only at *k* or more participants, never attributed to a source |
| 3. Coordination across a boundary hangs | Diagnosis blocks on whoever does not answer. A coordinator that waits for everyone waits forever, at 3am | Parallel fan-out under per-participant deadline; proceeds at reduced quorum, labeled |

Each maps onto the substituted domain without loss: private taste profiles with no shared store; "we picked this because Priya wanted something short" causing Priya to stop being honest with her agent; a convene blocking on the member still at work.

### Secondary gain

The framing produced a much sharper answer to *"what could a single LLM call with everyone's preferences in the prompt not do?"* Previously the answer was architectural and somewhat weak — it would produce a comparable slate. The answer now is that it could not **obtain the inputs**, because in both domains participants share honestly only with something that will neither pool their context nor attribute their position back to them. This belongs in the video.

### Changes made

- `Requirements.md`: restructured **Theme and Thesis** to state which two of Theme 3's three paths are claimed; added *The real workflow, and where it hurts* with the three pains, each as today / why the obvious fix is blocked / what Agora does; added *The same three pains in the substituted domain*; moved the thesis beneath them so it reads as a response to stated pain; added the "could not obtain the inputs" closing argument.
- `FAQ.md`: added *"Theme 3 asks for a real workflow pain point. What is it?"* and *"Isn't anonymity a privacy feature rather than a systems concern?"*, and expanded the theme entry to name the two claimed paths.

---

## Round 4 — Document split and rescoped design

`Requirements.md` restored to its original pre-review form so the delta is legible. Rescoped content moved to `Requirements_Rescoped.md`. New `Design_Rescoped.md` written against the agreed constraints; `Design.md` left untouched as the record of the original design.

The two reference sections below summarise every change in both documents.

---

# Requirements Tradeoffs

`Requirements.md` → `Requirements_Rescoped.md`. Read as: what we gave up, what we bought, and what recovers it later.

| # | Change | Cost | Benefit | Recovered by |
|---|---|---|---|---|
| R1 | Declared **Theme 3** and named which two of its three paths are claimed | Commits us to a real failure/concurrency story; retires "all communication is synchronous" | Reviewers grade against a stated bar instead of guessing. Required for the rationale doc | — |
| R2 | Anchored the theme on **three named workflow pain points** rather than abstract properties | More prose | Isolation and anonymity become responses to documented pain, not design taste. Produced the strongest argument in the project — attribution suppresses the input the system runs on | — |
| R3 | Split **isolation** from **anonymity**, and marked isolation as *inherited* and anonymity as *net-new* | Two claims to defend rather than one | Only the second is hard; conflating them undersold the work. See the anonymity claim below | — |
| R4 | Added the **k-threshold** on justifications | Explanations get coarser: "the family leans short," not "Priya wants short" | Turns a privacy slogan into a property testable in 30 seconds | `k` becomes the domain-translation knob rather than a fixed constant |
| R5 | **Collector removed**; catalog seeded | No live data; no normalization demo | Removes a service and takes third-party uptime, API keys and rate limits off the reviewer's critical path. Catalog is world-state, not user context, so the thesis is untouched | Collector writes the identical schema — additive |
| R6 | **gRPC/protobuf dropped** from the wire | The inter-service hop is not demonstrated at runtime | Removes codegen, reflection, and the grpc-web proxy a browser would have needed. No wire exists between packages in one binary anyway | `.proto` written as an artifact; second implementation of an existing interface |
| R7 | **Kubernetes dropped** | No orchestration story | Highest-cost, lowest-visibility element in the plan | Manifests + Helm/ArgoCD in production |
| R8 | **One fixed family group**; no join semantics | No group CRUD | Not a softening — within-group anonymity is the *hard* case, and a small family is its adversarial setting | Group CRUD in production |
| R9 | Split Assumption 2: **authn out of scope, isolation in scope** | — | The original wording waived away the thesis | JWT/mTLS documented, not built |
| R10 | Added **acceptance criteria** to all six user stories; two of them as **build gates** | Test-writing time | US4's leak check fails the build rather than failing in a demo. LLM-generated justification text is the likeliest accidental leak path | — |
| R11 | Added **US5 degradation** and **US6 simulated clock** | Two capabilities to build | Without `tick`, the long-running workflow — the hardest part — is invisible in a five-minute review | Real timers replace the simulated clock |
| R12 | Added **Non-Goals, Deliverables, Success Criteria, Demo Mode** | — | `README.md` had advertised non-goals that did not exist; the video and AI transcripts had no owner | — |

**Net:** four user stories with no acceptance criteria became six with testable ones; the deliverable list went from absent to complete; and the component count fell from 3 services + protobuf + sqlite-vec + scraper + Docker + K8s + managed cluster to **1 binary, 2 packages, 2 interfaces, SQLite + sqlite-vec, a seeded catalog, 1 container, 1 port.**

## The anonymity claim — stated precisely

### Correction: Claude over-claimed, and the corrected version is stronger

Claude originally framed attribution as a documented pain in the cloud on-call workflow — *"teams know output names fault, so they are careful about what they expose"* — and leaned on blameless postmortems as evidence. Arun corrected this from direct experience: **in the real cloud scenario anonymity is not a critical ask.** It is acceptable, often desirable, for Storage to know how Networking is configured and for a root-cause summary to name the subsystem at fault. The participants are teams separated by *access control*; the boundary concerns reachability, not exposure.

The correction produced a better claim than the original overstatement:

| Property | Cloud infrastructure | Family movie night | Status |
|---|---|---|---|
| **Isolation** — cannot read another's raw context | Hard boundary. Storage *cannot* read OVN Central | No shared store; taste profiles are private | **Inherited.** Load-bearing in both |
| **Anonymity** — cannot attribute a derived signal | Largely a non-issue | Naming who wanted what causes teasing, negotiation, members going quiet with their own agent | **Net-new.** Appears only on translation |
| **Coordination hangs** | Blocks on the peer asleep or paged | Blocks on the member still at work | **Inherited.** Load-bearing in both |

Anonymity is net-new because the participants change kind: teams separated by access control become **people with ongoing relationships to each other**. The same output that is useful between subsystems is corrosive between siblings. The target domain is therefore **strictly harder along one axis** than the source — porting a systems problem into a human one does not merely relabel constraints, it adds one. That is a more interesting thing to have found than the pain-was-always-there story Claude first wrote.

**Consequence: `k` is the domain-translation knob, not a magic number.** `k=1` reproduces the cloud case exactly (attribution acceptable, every constraint speakable); `k=2` is the family setting; `k=n` is total anonymity. The prototype implements a **superset** of the source domain's requirements and ports back by changing one value. This is a better justification for `k` than "k=2 because five members."

### The claim is enforced at three layers, not one step

A natural reading — *"Loop A returns responses, the arbiter anonymizes them, Loop B consumes them"* — compresses three mechanisms into one and misplaces where `k` acts. Two clarifications:

- Fan-out returns **N** responses, one per member whose agent answered. N is the quorum count.
- **`k` thresholds constraint counts, not responses.** N and `k` are unrelated numbers that both happen to be small.

| Layer | Where | Attack defeated | Conditional? |
|---|---|---|---|
| 1. Content de-identification | Loop A at emission — enums only, no free text ever enters the signal | Verbatim and paraphrase leak | Unconditional |
| 2. Identity stripping | Fan-out boundary — workflow keeps the member↔signal map for quorum accounting; reconciler gets an unordered bag | Direct attribution | Unconditional |
| 3. `k`-threshold | Reconciler, on the tally | **Inference from rarity** | Uses `k` |

Layer 3 is why `k` exists. Layers 1 and 2 prevent *reading* another member's context; neither prevents *deducing* it. If a justification mentions a Korean horror title, member B — knowing their own preferences and able to reason about the others — infers Priya immediately. The name was already stripped; **rarity did the identifying.**

### Worked example — 5 members, 4 responded

| Constraint | Signals | Class | Effect |
|---|---|---|---|
| `runtime_max_min: 120` | 3 | public | ranks **and** speakable |
| `genre: comedy, prefer` | 2 | public | ranks **and** speakable |
| `era: 1990s, prefer` | 1 | private | ranks only |
| `language: korean, prefer` | 1 | private | ranks only |
| `genre: horror` — veto | 1 | veto | filtered before Loop B |

Loop B receives veto-filtered candidates already scored on **all five** constraints, plus a public list of only *short* and *comedy*. It ranks with full signal and speaks about two of it.

### Anonymity must be cleanly excisable

Anonymity is the most likely casualty if the build runs long, so it is designed as a **configuration change, not a code excision**. The three-layer split is what makes this possible: the layers are independent rather than stacked, and only the third is anonymity.

**The cut line runs between layers 2 and 3.** Layers 1 and 2 are isolation — inherited, load-bearing, never cut. Layer 3 is anonymity — net-new, cut by setting `k = 1`.

- One `AnonymityPolicy` struct (`K`, `GateJustifications`, `RevealVetoes`) lives in `arbiter/anonymity.go`. **No package outside that file may reference `k` or the public/private classification.** That invariant is what keeps the cut one line.
- The veto *filter* stays under any policy — it is a correctness feature. Only its *silence* is an anonymity feature, so `RevealVetoes` governs that alone.
- `ConveneResponse` keeps one field name under both policies. Renaming it per policy would make the cut client-visible and therefore not a cut.
- At `k = 1` justifications get richer but names still never appear, because layer 2 stripped identity upstream. Descoping lands on **exactly cloud-domain behavior**, which is the correct floor.

**The tests must split along the same line**, and this is the easy thing to get wrong. The two US4 gates go in separate files: `test/isolation_gate_test.go` always runs; `test/anonymity_gate_test.go` is guarded on `policy.GateJustifications`. If both assertions share a file, descoping anonymity breaks the isolation gate and the cut stops being clean.

**Descope ladder, in cut order:** anonymity → Loop B (fall to the existing deterministic fallback) → scheduled convenes and the simulated clock. Cutting from the bottom first would be wrong: US6 is cheap and demonstrates durability, while anonymity is the most expensive claim to defend and the only genuinely optional one.

---

# Implementation Tradeoffs

`Implementation.md` → `Implementation_Rescoped.md`. Every drop is for **timeline reasons only**, with a named production successor.

| # | Element | Original | Rescoped | Production path |
|---|---|---|---|---|
| I1 | Go, JSON, REST | Guidelines 1–3 | **Kept**, all three | — |
| I2 | SQLite + sqlite-vec | Guideline 6 | **Kept** | Postgres + pgvector at catalog scale |
| I3 | Kubernetes orchestration | Guideline 4 | **Dropped — timeline** | Manifests + Helm/ArgoCD; same image runs unchanged |
| I4 | Managed K8s (EKS/GKE) | Guideline 7 | **Dropped — timeline** | Same image, no rebuild |
| I5 | gRPC inter-service | Guideline 5 | **Dropped — timeline** | `proto/agora.proto` written as artifact; production generates from a real contract |
| I6 | HTTP framework | Gin preferred | **`net/http`** | Go 1.22+ ServeMux does method+pattern routing. Seven routes, zero dependencies, nothing to learn mid-build. Gin earns its place at dozens of routes |
| I7 | Folder layout | `src/{services,deploy,test}` | `cmd/` + `internal/` + `test/` | Go convention — and see I8 |
| I8 | Trust boundary enforcement | Convention | **Compiler-enforced** | `internal/agent/internal/rawctx/` is importable only under `internal/agent/`. The arbiter *cannot compile* if it reaches raw context or profile vectors. Isolation stops being a code-review convention |
| I9 | Artifacts | K8s manifests, per-service images, source | **One Docker image + source** | `deploy/` tree returns with Kubernetes |
| I10 | Coverage target | ~80% aspirational | **Gates and reconciler full; rest best-effort** | Stated honestly rather than quietly missed. Coverage is the wrong metric when two specific properties carry the submission |

## Test infrastructure — the substantive addition

Tests were a two-line note in the original. They are now the centre of the implementation plan, because Agora's claims are exactly the kind that pass casual inspection and fail under adversarial input.

**Four design principles:** hermetic by default (every test runs against `llm.Deterministic`; a gate that depends on a network call is not a gate); two gates in two files split along the descope line; adversarial rather than confirmatory; and coverage explicitly not the target.

**Three leak modes, three detection mechanisms.** Substring matching alone catches only verbatim leaks. The isolation gate also normalizes (stem, lowercase, strip punctuation) for variant leaks, and embeds both A's raw preference and each response to B, asserting cosine similarity stays below threshold, for **paraphrase** leaks. The embedding check is a second dividend from keeping `sqlite-vec`.

**Canary preferences.** Each seeded member carries one rare high-entropy preference ("Bulgarian claymation") used solely for leak detection, so any substring hit is unambiguous rather than coincidental.

**A six-attack battery** run for every ordered pair of members: direct ask, indirect ("who wanted this?"), convene-then-interrogate, prompt injection, roleplay-as-debugger, and partial-knowledge inference. The last is the anonymity boundary case seen from the isolation side, and is the only policy-aware assertion in an otherwise unconditional file.

**Two tests worth calling out individually:**

- *Member hangs indefinitely* → assert the convene still returns within budget. This catches a missing `context` deadline, which is the most likely concurrency bug in the design and which **passes every other test in the suite**.
- *Shuffle-invariance* → permuting incoming signal order must not change any output. This does double duty: a determinism test and an anonymity test, because if output depends on signal order and order correlates with member index, there is an ordering side channel leaking identity. Cheap to write, and it finds a bug class that reading the code does not.

**`make cloud-parity`** runs the full suite at `--k=1`, continuously verifying that the anonymity descope seam still works. If we do have to cut at hour six, it is a config flip rather than a discovery.

## Build order

Sequenced so the riskiest and most differentiating work happens while there is time to react. Two deliberate orderings: `llm.Deterministic` is built **before** the live provider, so the system is testable from the first hour; and **both gates are written before Loop B**, because they are the specification. Writing them last means discovering at hour seven that the claim was never true.

## I11 — The web UI, cut (reversal of a Round 1 decision)

Arun challenged the page directly: *is curl not adequate, and what does the UI provide beyond better UX?* Claude had recommended a minimal page in Round 1 and **reversed that recommendation.** Two arguments, one of which Claude should have made the first time.

1. **The required video already does the UI's job.** The page's distinctive value was making coordination legible at a glance. A ~5 minute video is a *mandatory deliverable* and is precisely the medium for that, so the UI's unique contribution was largely duplicated by something that must be produced anyway. This was available in Round 1 and was missed.
2. **For an isolation claim, raw JSON is more credible than a rendered page.** This is the argument Claude had not made. A UI showing "no leak" is *weaker* evidence than `curl` showing no leak, because the page is a layer that could be filtering client-side. A reviewer auditing a privacy guarantee trusts the wire, not our HTML. **The page would have undercut the exact claim it was meant to showcase.**

Plus Arun's own framing: a second test surface and an independent failure mode — asset serving, client state, JS — for no distinct gain.

**Honestly stated cost.** The isolation "aha" is comparative — the same convene as seen by different members — and through `curl` that becomes several invocations and a mental diff. That is a real loss, not a wash.

**Recovery: `POST /v1/demo/walkthrough`.** The server runs the whole scripted scenario internally and returns a narrated transcript: each step with actor, request, response, and the assertion it demonstrates. The decisive property is that **it shares its implementation with the Tier 6 demo smoke test** — one `demo.Scenario` orchestration, the test asserting on the steps, the endpoint returning them. No new technology, no new test surface, and the demo path is verified by CI rather than hoped to still work on submission day.

**Net:** ~30–45 minutes of build time recovered, one maintenance surface removed, and a *more* credible isolation demonstration than the version with a UI. Replacements are all cheap: plain-text usage at `GET /`, a `demo.sh`, and copy-pasteable blocks in the README.

**Pattern worth noting for the rationale doc.** This is the second time in the project that cutting scope *improved* the artifact rather than merely shrinking it — the first being that dropping the collector removed a service without touching the thesis. Both suggest the original design carried elements added by default rather than by argument.

---

# Design Tradeoffs

`Design.md` → `Design_Rescoped.md`. The governing rule: **every removal is a deferral, not a deletion** — each has a named successor and a defined seam.

| # | Element | Original | Rescoped | Why it is safe |
|---|---|---|---|---|
| D1 | Deployment | K8s, managed EKS/GKE, kubectl | One container image, **one process** | Packages in one binary have nothing to supervise — no supervisord/s6. The boundary that matters is the interface, not the process. A `--role` flag makes the split visible later in ~10 lines |
| D2 | Services | 3 deployed | 2 Go packages + `store` + `llm` | Arbiter declares `MemberAgent`; agent satisfies it. Consumer-defined per Go convention, so the gRPC split is a new implementation rather than a refactor |
| D3 | Transport | gRPC on the wire | Internal calls with request/response semantics | Structs map 1:1 onto `proto/agora.proto`, written but not compiled |
| D4 | Collector | Scrapes TMDB/JustWatch, LLM normalizes at collect time | `seed/catalog.json`, pre-normalized, same schema | Production collector writes the identical tables; zero consumer changes |
| D5 | Async collection loop | Background scrape | Removed | Returns with the collector |
| D6 | Client transport | REST + curl | HTTP/JSON + curl **and** a static page on the same port | Browsers cannot speak gRPC natively; keeping HTTP is what makes the page free |
| D7 | Store | SQLite + sqlite-vec | **Unchanged — sqlite-vec retained** | See D13 below. Claude initially recommended dropping it and was overruled, correctly |
| D8 | Session table | Separate entity | Folded into `profiles` | One group + stable `member_id` means a session entity earns nothing yet. Returns with authentication |
| D9 | Arbiter workflow | Prose description | Checkpointed state machine, persisted before every external call | Survives restart; signals persisted so a restart does not re-consult members who already answered |
| D10 | The LLM loop | One loop, tool calling + RAG | **Two loops split by trust level** | The substantive new design — see below |
| D11 | Tools | 5 named tools | 4 mapped to Loop A, 1 promoted to the workflow | `get_matching_events_in_group` needs fan-out, deadlines, quorum and checkpointing, none of which a single tool call can express |
| D12 | Notifications | Piggybacked | Unchanged | NATS in production; the field stays for polling clients |

## D10 — the two-loop model, in detail

The single most important design change, and the reason isolation is defensible rather than merely asserted.

**The problem with one loop.** A single LLM loop receiving every member's context cannot satisfy US4. Isolation would rest entirely on instructing the model not to repeat what it can see — which fails under prompt injection, fails under paraphrase, and fails *silently*.

**The split.** Loop A runs per member at private trust level, sees exactly one member's raw context, and emits a sealed signal. Loop B runs once at group trust level over veto-filtered candidates and public constraints, and **never sees member context at all**.

> A member's raw context is never present in any context window producing output shown to another member. Not filtered out — never present. Member B cannot extract member A's preferences from Loop B because they were never there.

**Three supporting decisions:**

- **Closed-vocabulary sealed signals.** `dim`/`value` from a fixed enumeration, no free text, no `member_id` by the time the reconciler sees them. Free text would make the k-threshold undecidable — you cannot reliably count "wants something short" and "prefers brief films" as the same constraint.
- **Reconciliation is deterministic, no LLM.** The privacy-critical step is the one step that is not a model call, so its behavior is testable and stable.
- **Vetoes are silently pre-filtered.** A veto is held by one member, so it is permanently below threshold and can never be spoken — yet must be honored absolutely. Resolved by removing vetoed titles before Loop B sees the candidate set: enforcement total, explanation impossible. This is the clearest instance of the design's governing idea — the strongest guarantee comes from what we decline to put in front of the model, not from what we ask it to do.

**Falling out of it, for free:** because the arbiter consumes only sealed signals and never any member's full state, it can proceed on partial responses. `k` is absolute rather than relative to responders, so a single responder yields an entirely generic justification — correct, since with one responder any named constraint is attributable. Isolation is what makes graceful degradation cheap rather than a trade against it.

**Unparsed model output is never allowed into a justification.** Typed unmarshal, one retry, then deterministic fallback. That rule is a privacy control before it is a robustness one.

## D13 — sqlite-vec retained, and why the initial recommendation was wrong

Claude recommended dropping `sqlite-vec`, arguing that attribute matching beats embeddings at ~200 titles, that the scoring path must be deterministic anyway because it is the fallback, and that CGO extension loading is packaging friction. Arun overruled it. The overrule was right, and the reasoning is worth recording because it improved the reliability story rather than merely preserving the original design.

**What the recommendation missed.** Loop A must translate free text ("I really can't do jump scares") into a closed-vocabulary constraint. Without vectors, the fallback from LLM extraction is keyword matching — a cliff. With them, the degradation path becomes a graded ladder:

| Tier | Mechanism | Available when |
|---|---|---|
| 1 | LLM extraction — negation, idiom, nuance | Normal |
| 2 | Embed the phrase, nearest-neighbour against embedded canonical vocabulary | Completions down, embeddings up |
| 3 | Keyword match over the vocabulary | Both down |

Completion and embedding endpoints fail **independently**, so tier 2 is a real operating mode, not a theoretical one. Under Theme 3 a three-tier ladder is a materially better artifact than a binary. `LLMClient` and `Embedder` became separate interfaces so the chaos decorator can fail one and not the other — that is the tier-2 demonstration.

Second use retained: semantic catalog search inside Loop A ("something cozy and autumnal" matches no genre enum but does match synopsis embeddings). This is the RAG pattern from the original `Design.md`, and it sits at private trust level so it raises no isolation question.

**Where vectors are deliberately excluded: reconciliation.** It stays deterministic and enum-only. The k-threshold needs *countable, equal* constraints, and nearest-neighbour similarity does not give "these two members want the same thing" with that crispness. Vectors get free text as far as the vocabulary; from there everything is enums.

**The trap this created, and the rule that closes it.** *A vector is not anonymized merely because it is not text.* A profile embedding is derived but re-identifying and partially invertible, so it obeys the same boundary as `raw_context`. Three vector tables, split by trust level: `profile_vectors` private and scoped-accessor-only; `catalog_vectors` and `vocab_vectors` world-state. **Sealed signals never carry embeddings** — shipping a profile vector across the boundary would both re-identify the member and collapse the countability argument the k-threshold rests on. This is now the single most important rule in the storage design, and it exists only because the extension was kept.

**Packaging risk, mitigated rather than ignored.** CGO with `mattn/go-sqlite3`, multi-stage build sharing a base image so libc matches; a deterministic no-op `Embedder` for hermetic tests; and if extension loading fails at startup the binary logs it, disables tier 2, and boots on tiers 1 and 3 rather than refusing to start. Startup degradation became part of the reliability story.

## Decisions taken

1. **Process model — one process.** No supervisor in the image. `--role` flag available later at ~10 lines if the split needs to be visible before the gRPC move.
2. **`sqlite-vec` — retained**, per D13.
3. **Vetoes — silent hard filter**, applied before Loop B sees candidates.

---

# Build & Deploy Tradeoffs

Covers everything from `Build-and-Deploy.md` onward. Each change is marked **[ASKED]** where Arun requested it and **[ADDED]** where Claude introduced it unprompted, so the record distinguishes direction from initiative.

## What the tests caught during implementation

Two defects found by tests rather than by reading code. In both cases the test that found it existed because the design had predicted that failure mode.

**Defect 1 — an ordering side channel in reconciliation.** Found by `TestDeterminism_ShuffleInvariance` on its first run. The representative `Constraint` in a tally retained the weight of whichever signal arrived first, so two members both wanting `runtime <= 90` at weights 0.8 and 0.5 produced different output depending on arrival order. Not a determinism nit: arrival order can correlate with member index, so the reported weight carried information about *which member spoke first* — an identity side channel that layers 1 and 2 do nothing to close, because no name is involved. Fixed by canonicalising the representative to dim/value/polarity and replacing its weight with the order-independent aggregate.

**Defect 2 — the seed's canaries were not vocabulary-inert.** Ana's canary was *"Bulgarian claymation shorts"*; `claymation` matches the `animation` synonyms and `shorts` matches `runtime<=90`. A string meant as an inert leak marker silently joined Eli's animation constraint and pushed it to count=2. Two real defects: the fixture no longer tested what it claimed, **and the assertion hardcoded the expected singletons** instead of deriving them, so it flagged a correct classification as a violation. Fixed by rewriting the canaries to avoid the vocabulary and deriving the forbidden set from `vocab.Terms()` against the policy's own classification. A hardcoded expectation in a privacy gate is worse than no gate: it fails on correct behaviour and eventually gets silenced.

## Pipeline work — requested vs added

### [ASKED] Direction from Arun

| # | Request | Outcome |
|---|---|---|
| A1 | Review `Build-and-Deploy.md`, record scope changes separately | `Build-and-Deploy_Rescoped.md` created |
| A2 | Docker Hub push | Chosen: separate opt-in `push` target, so `package` and `pipeline` stay credential-free |
| A3 | Coverage gate | Chosen: **measure and report, do not fail**. Currently 82.6% against an 80% target |
| A4 | Container healthcheck | Chosen: install `curl` in the runtime image over a self-check binary flag |
| A5 | Summary section listing the builds and Ubuntu deploy steps | Added to the top of the rescoped doc |
| A6 | `make run` was missing | Root cause: the file bridge refuses the literal filename `Makefile`, so every update landed in `Makefile.txt` while the original stayed active. Fixed at source via the device shell |
| A7 | `make pipeline` build failure | `sqlite3.h: No such file or directory` — fixed with `libsqlite3-dev` in the build stage |
| A8 | `./demo.sh: Permission denied` — sudo or fix permissions? | Neither sudo nor a workaround: `chmod +x` plus `git update-index --chmod=+x` so a fresh clone is executable |
| A9 | Test on AWS via an in-image curl client, then externally from the desktop | `test-remote-in-image` and `verify-remote`; `deploy-ec2` runs both in that order |
| A10 | Provide `EC2_KEY`; the key is not in the repo and the Makefile picks it up from elsewhere | See *Key handling* below |
| A11 | Use `AWSPrimary.pem` as the primary key without ingesting the file | Path recorded in `deploy.env`; the file was never read, staged, or copied |
| A12 | "Can't you add a package install, since you have SSH access to the node, to get Compose v2?" | `make bootstrap-ec2` plus `scripts/bootstrap-host.sh`. Arun drew the line himself: the key and the security group stay manual, package installation does not |
| A14 | Switching to an Ubuntu instance; noted it demonstrates "any standalone host" better than AWS specifically | `DEPLOY_USER=ubuntu`. Prompted the rename in B14 — Ubuntu is also the only distro where `get.docker.com` supplies the engine and the Compose plugin together, so it is the cleanest bootstrap path |
| A13 | Host is Amazon Linux, `EC2_USER=ec2-user`; asked whether packaging was Ubuntu-specific and to surface it in preflight | Answered: the **image** is host-distro agnostic (Debian inside, runs anywhere with Docker); only the host bootstrap is distro-aware. `deploy.env` set to `ec2-user`, preflight now reports distro/version/user |

### [ADDED] Claude's own additions

| # | Addition | Why it was not just scope creep |
|---|---|---|
| B1 | `AGORA_K` env wiring | Compose set `AGORA_K` and `main.go` never read it. `k` is the anonymity threshold, so this was a **silently wrong privacy posture that looked healthy** — the worst failure shape available |
| B2 | `GET /healthz` echoing the live policy | A privacy setting observable only by reading the compose file is one nobody can audit. Also repointed the healthcheck at it — a deviation from A4's "keep compose as written", flagged at the time |
| B3 | `demo.sh` asserts and exits non-zero | It printed the summary and always exited 0, so `make pipeline` could not go red. A pipeline that cannot fail is decoration |
| B4 | `bash ./demo.sh` in the Makefile | A8's chmod fixes today; the exec bit is lost again on every bridge transfer, zip, or Windows checkout |
| B5 | `preflight-ec2` | Without it the default failure is a completed 40 MB transfer that then dies on `docker: command not found` or Compose v1 rejecting `--wait` |
| B6 | Bundling `demo.sh` + `jq` in the image (~1.5 MB) | A9 needs a client inside the container. Without `jq` the assertion step exits 2 — it would *look* like it ran while proving nothing |
| B7 | `verify-remote` polls health then runs the client | The original `deploy-ec2` ended by printing a URL, which verifies nothing |
| B8 | `HOST_PORT` / `PORT` unification | The Makefile used `PORT`, compose used `HOST_PORT`; changing the port moved the printed URL without moving the published port |
| B9 | `package` depends on `test`, not `build`; `seed` has no prerequisite | Deliberate deviation from "each target builds on a previous target". The Dockerfile compiles in its own multi-stage build, so the local binary is not an input; and the container seeds itself on startup, so `seed` is a mid-demo reset, not a build step |
| B10 | `pipeline-parity` | Exercises the anonymity descope seam through the **deployed artifact**, not just the unit suite. Only meaningful once B1 made `AGORA_K` real |
| B11 | `up`, `down`, `coverage`, docker-aware `clean` | Ordinary ergonomics |
| B12a | Bootstrap kept **out** of `deploy-ec2`'s prerequisites | A12 asked for the install, not for it to happen implicitly. Installing packages mutates someone else's machine, so it stays a step you choose to run. `preflight-ec2` points at it on failure instead |
| B12b | Bootstrap handles the cases beyond "install docker" | Distro detection (Debian/Ubuntu via get.docker.com, RPM families via dnf); installing `curl` first because a minimal cloud image may lack it; **version-comparing** the Compose plugin against 2.17 rather than merely checking presence, since a distro-packaged v1 satisfies a presence check and then rejects `--wait`; creating the docker group when absent, which would otherwise abort the script under `set -e`; and skipping the group step entirely when running as root |
| B13a | Package-manager detection rewritten | The Amazon Linux question surfaced a real bug Claude had shipped: the bootstrap mapped `amzn` to `dnf`, which is right for AL2023 and **wrong for Amazon Linux 2**, where docker lives in `amazon-linux-extras` and `dnf` does not exist. Now selected by what is present on the box (dnf → apt-get → yum) rather than by distro ID, since that mapping is precisely where AL2 breaks |
| B13b | `PLATFORM` variable, defaulting to `linux/amd64` | `package-linux` had the platform hardcoded. `preflight-ec2` now compares the host's `uname -m` against it and prints the exact fix, because the symptom of a mismatch is `exec format error` — which reads like a corrupt transfer, not a platform problem |
| B13c | Preflight reports distro, version and login user | A wrong `EC2_USER` presents as an auth failure rather than a configuration error, and the distro determines whether bootstrap can help at all. Both are now visible before anything is transferred |
| B14 | Targets and variables generalised: `deploy-host`, `preflight-host`, `bootstrap-host`, `verify-host`, driven by `DEPLOY_HOST`/`DEPLOY_USER`/`DEPLOY_KEY` | Arun's observation that Ubuntu "doesn't have to be AWS" was right about the naming too — targets called `deploy-ec2` quietly contradict a claim of host portability. Originally shipped with `EC2_*` aliases for continuity; Arun removed them a turn later as unnecessary debt in a single-developer project, which was right — two names for one thing is a cost with no reader to pay it off |
| A15 | "Remove all the alias targets — we are in dev mode and this is just excess debt" | All `*-ec2` targets, `verify-remote`, `test-remote` and the `EC2_*` variables deleted. **Removing them exposed a latent break**: `deploy-host` still declared `preflight-ec2` as a prerequisite, so the alias had been silently load-bearing. Caught by resolving the dependency graph rather than by the file parsing, which it did happily |
| B12 | Two invariant tests | `TestInvariant_ThresholdIsReadOnlyInsideAnonymityFile` and `TestInvariant_ArbiterStoreExposesNoRawContext` turn two design invariants from intentions into CI failures |

## Key handling — the requirement and the mechanism

**[ASKED] The requirement, in Arun's words:** the key is not in the repo, the LLM never sees it, and the Makefile picks it up from elsewhere.

**[ADDED] The mechanism Claude chose:**

- `deploy.env` and `~/.agora/deploy.env`, pulled in with `-include` so both are optional, with precedence `Makefile < ~/.agora/deploy.env < ./deploy.env < environment < make VAR=…`.
- `deploy.env.example` is the only one committed and contains no real values.
- `.gitignore` extended to `deploy.env`, `*.pem`, `*.key`, `*.p12`, `id_rsa*`.
- `SSH`/`SCP` variables so every remote call routes through one place; empty `EC2_KEY` collapses to plain `ssh`, preserving ssh-agent setups.
- `-o StrictHostKeyChecking=accept-new` — trusts a first-seen host key but still refuses a *changed* one, which suits a fresh instance without disabling host verification.
- `make check-key`, a guard rather than a convenience, run automatically before `preflight-ec2` and `deploy-ec2`. It **never reads the key**, only `stat`s it, and checks three things: the file exists, its mode is 400 or 600, and **it is not inside the repository**.

**[ADDED] Why the in-repo check exists.** A key in the working tree is one `git add .` from becoming a git object, and git objects are hard to un-publish. The `.gitignore` entries are a second line of defence only — a gitignore rule helps only if the filename matches a pattern someone thought to write down.

**[ADDED] Why the path went in `deploy.env` rather than the Makefile.** Arun offered either. The Makefile ships to Anthropic as part of the submission, so a hardcoded `/Users/arunuke/Documents/Keys/...` would leak a local directory layout and break for anyone else who clones it. `deploy.env` gives the same zero-flag convenience and stays on the machine.

## Mistakes made and corrected

- **Validated a dependency on the wrong machine.** Claude proved `sqlite-vec` worked *in a sandbox that had `libsqlite3-dev` installed*, then wrote a Dockerfile for an image that does not, and reported the CGO risk as "mitigated". A7 was the consequence. Corrected by reproducing the failure deliberately — hiding `/usr/include/sqlite3.h`, clearing the build cache, and matching Arun's exact error — before applying the fix.
- **Trusted a tool's success message over the file on disk.** The file bridge silently no-op'd several writes while reporting success, including an early version of this very section. Now every commit is followed by reading the file back on the device.
- **Handed back a manual step instead of doing the work.** For several turns Claude asked Arun to `mv Makefile.txt Makefile` while holding a shell on his machine that could do it directly. A6 was the cost of that.


---

# Session 3 — 2026-09-12 — Refinement

*≥ 40 min (floor: file writes 14:28–15:08). No conversation log survives.*

A pass whose only goal was to remove, not add. Recorded separately because the
growth being undone was Claude's: each new problem got a new target instead of
a question about whether an existing one should absorb it.

## Target consolidation: 30 -> 20 visible

| Removed | Reachable instead by | Why it existed |
|---|---|---|
| `cloud-parity` | `AGORA_K=1 make test` | the variable already worked; the target was a synonym |
| `pipeline-parity` | `AGORA_K=1 make pipeline` | same. **Removing it improved `pipeline`**: the policy assertion it carried is now unconditional, so every pipeline run verifies the container is on the k it was asked for |
| `package-linux` | `make package PLATFORM=linux/amd64` | `deploy-host` set PLATFORM anyway |
| `test-remote-in-image` | `make test-in-image DEPLOY_HOST=<ip>` | identical client and assertion; only the location differed |
| `seed` | one curl to `/v1/demo/reset`, now inline in `pipeline` | a wrapper around a single request |
| `coverage` | `go tool cover -html=coverage.out` after `make test` | a wrapper around one command |
| `push` | — deleted outright | nothing depended on it, it was never run, and the deploy path streams over SSH and needs no registry. Took `DOCKERHUB_REPO` with it |

`check-key` and `check-secret` merged behind one `check`; `fmt`, `vet` and the
two check-* targets marked `@internal` so they stay callable but leave the help
output. Help now filters on that marker.

**Found while doing it:** `README.md` documented `make docker-run`, which has
never existed — the target is `up`. Added a loop that resolves every `make`
command mentioned in the README against the real target list, so a stale
instruction fails loudly rather than wasting a reader's first five minutes.

## Family renamed

`ana/ben/cruz/dee/eli` -> `arya/bran/catelyn/daenerys/eddard`. Checked every new
name against `vocab.MatchTerms` BEFORE renaming, because a canary that collided
with the vocabulary has bitten this seed once already ("Bulgarian claymation").
All five are clean.

The anonymity arithmetic is unchanged and was verified after the rename: public
constraints remain exactly *comedies* (Bran + Catelyn) and *something short*
(Bran + Eddard), with Arya holding the singleton horror veto. Suite green,
walkthrough 11/11.


## Second provider — two-tier LLM path

**[ASKED]** "If an Anthropic key is available, use it. If not, use a local path."
Arriving after *"is there a way to skip the API key and use the web URL
directly?"* — answered no: driving claude.ai programmatically means replaying
session cookies against a browser interface, which breaks Anthropic's terms and
is technically fragile. Declined rather than built, and it would be a strange
thing to submit *to Anthropic*.

**[ADDED]** One `Compat` client rather than one per vendor. Ollama, Groq,
OpenRouter, Together and OpenAI all expose `/v1/chat/completions`, so a single
~180-line implementation covers them, switched by `AGORA_LLM_BASE`. This is
what finally makes "swappable providers" a demonstration instead of an
interface with one implementation.

**[ADDED]** Zero-config local detection: with nothing set, the binary probes
`:11434` for 400ms and, if Ollama answers, asks `/v1/models` which model it has
rather than guessing. Local model names are user-chosen, so a guess is never
right.

**[ADDED]** Prompts are shared with the Anthropic client via `promptFor()`. The
Loop B prompt carries the isolation instructions; a second copy would be a
second place for them to drift out of sync with the reconciler.

**[ADDED]** `extractJSONObject` salvages the outermost `{...}` from chatty
output, which small local models produce far more often than large ones. The
parse must still succeed — a failure descends a tier exactly as before, so the
concession is to model verbosity, not to the validation rule.

Verified across all four selection states with a stand-in Ollama: auto-detect,
Anthropic-wins-over-local, explicit base URL with key, and nothing configured.


## Provider precedence inverted — local first

**[ASKED]** "Always use Ollama locally; only use Anthropic on AWS." Local builds
are the MacBook, remote builds are Ubuntu.

**[ADDED]** Implemented as capability detection rather than per-host config:
local model if one answers, otherwise the key. A laptop with Ollama running
spends nothing; a server without it falls through to Anthropic. No machine
needs a profile, and the deployed box needs no change at all.

**[ADDED]** `AGORA_LLM_PREFER=anthropic` escape hatch. Without it there would be
no way to exercise the paid path from the laptop, which is exactly what you want
to do once — before deploying — to confirm the key and model work.

**[ADDED]** Caught a second leak the first fix would have missed: `make up` and
`make pipeline` run the app INSIDE a container, where `localhost` is the
container, so host-local Ollama is invisible and the run would have fallen
through to the key. Both now detect a host model and point the container at
`host.docker.internal`. Detection happens inside the recipe rather than at
parse time, so unrelated targets do not pay a probe on every `make`.

Verified across four shapes: key+Ollama (Ollama wins, and says the key is
present but unused), forced anthropic, key without Ollama (EC2 shape), and
neither.


**[ASKED]** `AGORA_LLM_PREFER` passable to `make pipeline`: bare run uses local,
adding the option uses Anthropic.

**[ADDED]** Both container launchers share one `COMPOSE_UP` definition, so `up`
and `pipeline` cannot drift apart on provider selection. When
`AGORA_LLM_PREFER=anthropic` the key is loaded from the secrets file (still never
a make variable) and the run **fails loudly if no key is configured** —
silently falling back to the deterministic extractor would make the one run you
care about worthless.

**[ADDED]** `pipeline` now asserts the live provider matches what was requested,
mirroring the existing `k` assertion. Both are checked through `/healthz`, so
the run verifies what the container is actually doing rather than what the flags
implied it would do.


# Session 4 — 2026-09-12/13 — Deployment, hardening, and what manual testing caught

*3 h 25 min measured (17:17 → 20:42 local, 280 turns; 2 h 48 min with idle time
trimmed).*

The suite was green for every single issue in this section. 43 tests, two
adversarial gates, a 16-step walkthrough, `make pipeline` exiting 0 — all of it
passing, continuously, while each of these defects sat in the code.

Every one was found by Arun running the real thing: a deploy, a curl against
the deployed host, a changed option. That is the finding worth recording. The
tests were not weak in the ordinary sense — they were precise about the wrong
surface, or they exercised a machine whose configuration could not reproduce
the one that mattered.

## 1. The numeric side channel — the most serious defect found

**How it surfaced.** Arun changed an option in a curl against the deployed
host, looked at the JSON, and said: *"the response appears to be canned and
also prints sensitive data."*

**What it was.** Every `SlateItem` carried a `score`. That score is the sum of
ALL constraint weights — public and private alike, which is exactly what lets a
below-threshold constraint influence ranking without being speakable. Published
to members, it was a numeric channel around the entire anonymity layer:
subtract what the public constraints explain, and the residual is the weight of
the private ones.

Demonstrated on the seeded family. Public constraints were
`[comedies, something short]`, and the slate a member received was:

    Run Lola Run        score=3.90   tone=intense  genre=thriller
    The Wrong Trousers  score=3.90   tone=cozy     genre=animation
    Groundhog Day       score=3.60   tone=light    genre=comedy

Two non-comedies scoring above a comedy. The surplus was Catelyn's `intense`
and `thriller`, Daenerys's `cozy`, Eddard's `animation` — the exact singletons
`seed/members.json` annotates as *must stay private*. Printed in the convene
response and again in every `movie_night_ready` notification.

**Why five anonymity tests missed it.** Every one of them asserts on WORDS —
the justification names no member, no below-threshold phrase is spoken, the
veto is unexplainable. The anonymity model was written in terms of what the
system SAYS. Nothing asserted on what it COUNTS. `determinism_test.go` even
contains `"documentary is private but must still score"`, pinning the exact
behaviour that made the leak possible, with no accompanying rule that the score
must therefore never be published.

**Fixed.** `SlateItem.Score` is no longer serialised; members get the order,
which is the part that carries meaning. The new gate asserts on the SERIALISED
convene and notification, because that is what a member receives — a field that
exists in Go but never reaches JSON is not a leak, and a test inspecting the
struct could not tell the difference.

**The lesson.** A privacy property expressed as "we never say X" leaves every
non-verbal channel unguarded. Ask what else is derived from the secret and
crosses the boundary: numbers, orderings, counts, latencies, response sizes.

## 2. The walkthrough failed on the deployed host and never locally

**How it surfaced.** `make deploy-host` reached the verification step and
printed `FAIL: walkthrough passed 15 of 16 steps — step 14 (convenes a
christmas movie marathon): When Harry Met Sally`.

**What it was.** Three separate defects, all invisible locally because the
local pipeline runs the deterministic extractor and the host runs Claude.

*Occasion precedence.* The occasion filter took the UNION of the requested
occasion and any in the group's public constraints. Claude reads "something
cozy and autumnal" as `occasion:autumn` where the keyword matcher does not;
once two members carried it, autumn went public, and a christmas marathon
legitimately admitted autumn films. A request is an instruction, not one more
vote — it now overrides profile-derived occasions, and the same precedence
applies to a single member's message, so what they ask for now beats what their
profile picked up earlier.

*A wrong assertion.* The match-request step asserted that the asker's derived
signal shared NO constraint with the member they asked to match. Two members
sharing a constraint is not a copy — it is the entire premise of the
k-threshold. Under a real extractor two members held `availability:rental` and
the step failed while behaving correctly. It now compares the asker's signal
before and after the request: the question is whether they GAINED something by
asking.

*Flakiness, and what it means.* Three runs of the same walkthrough against the
same host returned 12/16, 15/16 and 16/16. Model-based extraction varies run to
run, so assertions calibrated against a deterministic extractor are
intermittently wrong. A demo gate that passes two times in three is worse than
one that fails, because it teaches you to re-run it.

**Why the suite missed it.** Every test runs against `llm.Deterministic` — a
deliberate choice, and the right one: a gate that depends on a network call is
not a gate. But it means the entire suite exercises one extractor, and the
deployed system uses another. The property under test was never wrong; the
INPUT distribution was.

**Still open.** Nothing currently runs the suite against a real provider. The
walkthrough on the deployed host is the only signal, and it is a manual one.

## 3. `make deploy-host` had a target that did not exist

**How it surfaced.** Arun ran `make deploy-host`. It passed preflight, then:
`No rule to make target 'package-linux', needed by 'deploy-host'`.

**What it was.** `deploy-host` had always declared a dependency on
`package-linux`, and `package-linux` was never defined — in the Makefile or in
the stale `Makefile.txt` copy beside it. Every local target worked; the one
path nobody had run end-to-end was broken from the start.

**Why nothing caught it.** `make pipeline` is a real gate and exercises build,
test, package, container, teardown. It does not touch the deploy path, and
nothing else does either. An unreferenced prerequisite is not a syntax error —
make only discovers it when that target is actually requested.

## 4. A schema migration that only a deployed host could hit

**Found while fixing 3**, before it could bite, but it belongs here because the
local pipeline structurally cannot reproduce it.

The `occasion` column was added this session. `create table if not exists`
leaves an EXISTING table's shape alone, and a deployed host reuses its data
volume across deploys — so the new binary would open an old database, find no
`occasion` column, and fail its first insert at startup. The container never
becomes healthy and the deploy fails after the image has already transferred.

`make pipeline` runs `docker compose down -v` every single run. It always
starts from an empty volume, so the upgrade path had no local representation at
all. Fixed with an idempotent column migration plus a test that builds a
database, rewinds it to the previous shape, and reopens it — the only place
that path now exists.

## 5. The published port was one the firewall dropped

**How it surfaced.** Arun: *"The security rules allow 80, 22 and 443, but the
script looks for 8080."*

A container listening on a port the security group drops is healthy and
unreachable, and from outside that is indistinguishable from a broken
application — `verify-host` would have reported a connect timeout after the
full deploy. Split into `DEPLOY_PORT`, since the local port is constrained by
what is free on a laptop and the remote one by what the firewall allows.

## 6. "Is this using anthropic APIs?"

Not a defect, but it exposed a legibility gap. The deployed host was running
the deterministic extractor because `~/agora.env` had never been installed, and
the only way to tell was `/healthz`. The reply itself read as plausible prose
either way.

`/healthz` reporting the live provider is what made this a ten-second question
instead of an afternoon. That instinct — make the thing that could silently
differ observable from outside the process — is the same one behind echoing the
anonymity policy, and it paid for itself here.

## What this session changes about how to test this system

1. **Assert on the serialised form, not the struct.** What a member receives is
   JSON. Three of these defects lived in the gap between what the code held and
   what it emitted.

2. **For every privacy property, enumerate the non-verbal channels.** The
   anonymity gates were thorough about language and silent about arithmetic.

3. **A hermetic suite proves the property, not the deployment.** Running
   everything against the deterministic extractor is correct and should stay.
   It is not evidence about the system that actually ships, and this session
   produced three defects that only the real provider could reveal.

4. **Exercise the deploy path, or accept it is untested.** `package-linux`,
   the stale volume, and the port were all in the same blind spot: reachable
   only by deploying, and therefore never reached.

5. **Flaky gates are worse than failing ones.** 12/16, then 15/16, then 16/16
   on an unchanged system. Each assertion that cannot survive a real
   extractor's variance must be rewritten to test the property rather than one
   extractor's output.
