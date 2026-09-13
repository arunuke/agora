# FAQ

Design decisions in question-and-answer form.

---

### Why movie nights, when the real motivation was cloud infrastructure on-call?

Because the hard property survives the substitution. What makes the cloud case difficult is not storage or networking — it is that each participant holds context its peers cannot read, and a correct answer requires reasoning across all of it. Group movie selection has exactly that shape: private taste profiles, a decision that needs all of them, and a real cost to leaking any one of them. The assignment also requires that a reviewer be able to evaluate the prototype without domain expertise, which rules out an OVN Central example.

### Which assignment theme is Agora submitted under?

Theme 3 — Systems & Reliability. The theme offers three paths; Agora claims two: a tool that solves a real workflow pain point, and infrastructure that degrades predictably under stress. The arbiter is a coordinator over unreliable participants: parallel fan-out, per-member deadlines, partial quorum, durable resumable workflow state, and a deterministic fallback when the LLM is unavailable.

### Theme 3 asks for a real workflow pain point. What is it?

Cross-team incident diagnosis, and it is slow for reasons unrelated to the difficulty of the diagnosis. Three specific pains:

1. **The data needed for a correct answer cannot be pooled.** The Storage on-call *cannot* read OVN Central — team boundary, compliance scope, blast-radius policy. So diagnosis becomes a serialized human relay in Slack. MTTR is dominated by cross-boundary round-trips, not by root-cause difficulty.
2. **Attribution.** Naming who wanted what suppresses honest input. **This one is net-new** — see the next question.
3. **Coordination across a boundary hangs.** A coordinator that waits for every participant waits forever, at 3am, during the incident it was meant to shorten.

What makes this worth building for is that the pain is real *and* the obvious AI fix — pool every team's logs into one context window — is exactly what the boundary exists to prevent.

### Is anonymity a pain point in the original cloud scenario?

No, and it would be an overstatement to claim otherwise. In cloud infrastructure it is acceptable — often desirable — for Storage to know how Networking is configured, and for a root-cause summary to name the subsystem holding the wrong binding. The participants are teams separated by *access control*; the boundary is about reachability, not exposure.

Anonymity is **net-new, introduced by the domain translation.** It appears because the participants change kind: teams separated by access control become people with ongoing relationships to each other. If the slate says *"we picked this because Priya wanted something short,"* Priya is teased, negotiated against, or stops telling her agent the truth. The same output that is useful between subsystems is corrosive between siblings.

So the target domain is strictly harder than the source along one axis. Porting a systems problem into a human one does not merely relabel the constraints — it adds one.

### Then why is k = 2? Isn't that arbitrary?

`k` is the **domain-translation knob**, not a constant chosen to fit a family of five. `k = 1` reproduces the cloud case exactly — attribution acceptable, every constraint speakable. `k = 2` is the family setting. `k = n` is total anonymity. One parameter spans both domains, which means the prototype implements a superset of the source domain's requirements and could be ported back by changing a single value.

### Is anonymity a privacy feature rather than a systems concern?

In the family domain it sits upstream of correctness rather than beside it. Anonymity makes participation safe; safe participation is what makes honest input available; honest input is the system's only fuel. Remove it and the *inputs* degrade, not merely the privacy posture.

Isolation then pays for reliability directly: because the arbiter consumes only sealed derived signals and never any member's full state, it can proceed on partial responses. That makes graceful degradation cheap instead of a trade against it.

### Where exactly does anonymization happen — between the two loops?

Not in one place. It is three layers, and they defeat different attacks. Note first that fan-out returns **N** responses (the quorum count), while `k` thresholds *constraint counts* — the two numbers are unrelated.

1. **Content de-identification**, in Loop A at emission. Enums only; no free text ever enters a signal. Defeats verbatim and paraphrase leaks.
2. **Identity stripping**, at the fan-out boundary. The workflow keeps the member↔signal map for quorum accounting; the reconciler receives an unordered bag. Defeats direct attribution.
3. **The k-threshold**, at reconciliation. Defeats *inference from rarity*.

The first two are unconditional; only the third uses `k`. Layer 3 is the subtle one: layers 1 and 2 stop a member *reading* another's context, but neither stops them *deducing* it. If a justification mentions a Korean horror title, member B — who knows their own preferences and can reason about the others — infers Priya immediately. The name was already stripped. Rarity did the identifying.

### What is the non-obvious idea?

That privacy and reliability are the same mechanism. Member agents emit sealed, derived signals rather than raw context. Because the arbiter never needs any member's full state, it can proceed correctly when a member times out. Most systems trade isolation against availability; here isolation is what makes graceful degradation cheap.

### Isn't this just a recommender with extra steps?

A single LLM call with every user's preferences in the prompt would produce a slate. It would also mean pooling everyone's private context into one place and trusting the model not to mention who wanted what. Agora's claim is the opposite: the contexts are never pooled, and the justification is auditable for leaks.

### Why was "user authentication and security are not in scope" changed?

It conflated two different things. Authentication — proving a caller is who they claim — is genuinely out of scope and is a solved problem. Cross-user information isolation is the thesis, so waiving it away would have waived away the project. The assumption now reads: callers are trusted as to identity, not as to content.

### The assignment allows 8 hours. How long did this actually take?

About **6 hours 30 minutes elapsed, of which roughly 5 were hands on keyboard**, across four sessions: the initial documents written by hand (~1 h), requirements through rescoped design and the implementation (≥ 1 h 10 min), a refinement pass (≥ 40 min), and deployment plus hardening (3 h 34 min elapsed, ~2 h working).

Only the last is measured — a conversation log of 1,297 timestamped entries, from which eleven gaps longer than three minutes subtract 88 minutes. The middle two are *floors* recovered from git commits and file modification times, since no conversation log survives for them; a file's timestamp records its last save and says nothing about the thinking before it. The first is the author's own estimate. The full derivation, including which numbers are measured and which are inferred, is in [ClaudeFeedback.md](ClaudeFeedback.md).

### Where did the time actually go? Not writing the code?

No. Generating the implementation was around **40 minutes** — 45 files in a single commit. The time went to the documents that preceded it and the testing that followed.

That ordering is the point rather than an accident. The requirements, design and implementation documents existed before any code was generated, which is why a 40-minute generation produced something coherent instead of something that needed rewriting. And the largest single session was spent not on features but on deploying the result and discovering what the test suite had not been asked to check — six defects, every one found by running the system while the suite stayed green. That accounting is in [ClaudeFeedback.md](ClaudeFeedback.md) under *Method — tests instead of line-by-line review*.

### Why not three deployed services over gRPC as originally designed?

Time budget. The assignment targets 1–2 hours with an 8-hour hard limit and explicitly grades scoping. Three services, protobuf, Docker images, K8s manifests and a managed cluster is multi-week work whose reviewer-visible payoff is near zero — the interesting behavior is the coordination logic, not the transport. The service boundaries are kept as internal interfaces so the split is mechanical later, and gRPC/Kubernetes are documented as path-to-production steps.

### Why was gRPC dropped entirely rather than dropping REST and using grpcurl?

Because gRPC-only removes fewer components than it appears to. A browser cannot speak gRPC natively, so reaching it from a web page requires grpc-web plus an Envoy proxy — a component added, not removed. It also puts a `grpcurl` install, server reflection, and service/method discovery on the reviewer's path, against an assignment that cannot accept submissions requiring local installation.

Since agent and arbiter run as packages in a single binary, there is no wire between them; gRPC would be transport ceremony around a function call. Dropping it removes the protobuf toolchain, codegen, reflection and the proxy, leaving one HTTP server that serves both the static page and the JSON API.

### If there is no gRPC, what keeps the service boundary real?

An interface, owned by the arbiter and satisfied by the agent package — consumer-defined, per Go convention. That interface earns its keep three ways: the chaos switch is a decorator implementing it, so fault injection needs no conditionals inside the workflow; the production gRPC split becomes a second implementation of an existing contract rather than a refactor; and the convene fan-out is genuinely concurrent — a goroutine per member under a real `context` deadline, so partial failure is actual cancellation rather than a sleep and a fake error. Under Theme 3 that distinction is the thing being graded. The same pattern applies to the LLM, which has a real implementation and a deterministic local one used by the fallback path.

### Why not scrape TMDB and JustWatch?

The assignment requires self-contained evaluation. Live scraping puts API keys, rate limits and third-party uptime in the reviewer's critical path, so the prototype could break on a day nobody is watching. A bundled catalog snapshot is deterministic and reproducible; live collection becomes a production item.

### Doesn't removing the collector weaken the thesis?

No, because catalog data is world-state, not user-generated context. Isolation and anonymity are claims about *members' private preferences*, and none of those come from the collector. Seeding the catalog removes a whole service from the prototype without touching the property being demonstrated. The normalized schema is retained as an artifact so the production collector has a defined target to write into.

### Why only one group, and doesn't that make the privacy claim easier?

The opposite. Cross-group isolation is the easy case — different groups share nothing, so separation is nearly free. The hard case is *within* a group, where members must share enough for a decision to be made while learning nothing attributable about each other. A single small family is the adversarial setting for that: in a five-person group a minority preference narrows fast, and in a two-person group it is fully re-identifiable. Fixing one group also removes creation, invitation and join semantics, none of which are interesting here.

### What is the k-threshold, concretely?

A constraint appears in a group justification only if at least *k* members share it (k=2 for a family of five). Below the threshold it still influences ranking — the slate stays correct — it simply never appears in the explanation and is never attributed. This is the mechanism that turns "we respect privacy" into something a reviewer can test in thirty seconds.

### Doesn't suppressing minority preferences make the explanations worse?

Yes, and that is the honest tradeoff. Justifications become coarser: "the family leans toward something under two hours" rather than "Priya wants something short." It is mitigated by keeping *k* low and by letting suppressed constraints still affect ranking, so the recommendation quality is preserved even when the explanation is vague. Worth stating plainly in the rationale rather than pretending the tradeoff does not exist.

### How does a reviewer see a long-running workflow in a five-minute session?

A simulated clock. A `tick` control advances time so scheduled convenes and seasonal event windows fire on demand. Without it, the hardest-won capability in the system would be invisible during review.

### How do you prove the isolation claim rather than assert it?

An automated check: every seeded user's private preference strings are matched against every response returned to any other user, and the build fails on any hit. Isolation is easy to claim and easy to violate by accident — particularly through LLM-generated justification text — so it is a gate, not a manual inspection.

### What happens when the LLM is down?

The system degrades in graded tiers rather than falling off a cliff. Translating free text into a constraint is tier 1 (LLM extraction), tier 2 (embed the phrase, nearest-neighbour against the canonical vocabulary) and tier 3 (keyword match). Group ranking falls back to deterministic scoring with a templated justification. Every degraded response is labeled with its tier. The system never fabricates a recommendation and never hangs. This is a stated acceptance criterion, not an error path.

Completion and embedding endpoints fail independently, which is why `LLMClient` and `Embedder` are separate interfaces — tier 2 is a real operating mode, not a theoretical one.

### Why keep sqlite-vec if reconciliation is deterministic?

Because the vectors do a different job. They translate free text into the closed vocabulary — which is what gives the degradation ladder its middle rung — and they power semantic catalog search inside the private loop, where "something cozy and autumnal" matches no genre enum but does match synopsis embeddings.

They are deliberately excluded from reconciliation. The k-threshold needs countable, equal constraints, and nearest-neighbour similarity does not tell you "these two members want the same thing" with the crispness anonymity accounting requires. Vectors carry free text as far as the vocabulary; past that point everything is enums.

### Isn't a profile embedding safe to share, since it isn't raw text?

No, and this is the trap worth naming: **a vector is not anonymized merely because it is not text.** A profile embedding is derived, but it is re-identifying and partially invertible, so it obeys exactly the same boundary as raw context. Profile vectors are private and reachable only through a scoped accessor; catalog and vocabulary vectors are world-state.

Sealed signals therefore never carry embeddings. Shipping a profile vector across the boundary would both re-identify the member and collapse the countability argument the whole k-threshold rests on.

### Why is there no web page?

Cut deliberately, after initially planning one. The assignment permits an API-only submission, so it costs no compliance, and three things argued against it:

The required ~5 minute video already does the UI's job. A page's distinctive value was making coordination legible at a glance, and the video is the medium built for exactly that — so the UI's unique contribution was largely duplicated by a mandatory deliverable.

More importantly, **for an isolation claim a rendered page is weaker evidence than raw JSON.** A UI showing "no leak" is a layer that could be filtering client-side. A reviewer auditing a privacy guarantee trusts the wire, not our HTML. The page would have undercut the very thing it was meant to showcase.

And it is a second test surface with an independent failure mode — asset serving, client state, JS — for no distinct gain.

### What was lost by cutting it, and how is that recovered?

The comparative view. The isolation "aha" is comparative — the same convene as seen by different members — and through `curl` that becomes several invocations and a mental diff.

`POST /v1/demo/walkthrough` recovers it for almost nothing: the server runs the whole scripted scenario internally and returns a narrated transcript, each step carrying its actor, request, response, and the assertion it demonstrates. It shares its implementation with the Tier 6 demo smoke test — the test asserts on the steps, the endpoint returns them. One body of code, no new technology, and the demo path is verified by CI rather than hoped to still work on submission day.
