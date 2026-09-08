# TL; DR

Agora reconciles **private, mutually inaccessible user context** into a **group decision**, without any member's raw preferences leaking to another member — and without the group's justification revealing *who* wanted what.

It consists of an **agent** module that users talk to and that holds per-user private context, and an **arbiter** module that owns state, drives the LLM, reconciles sealed member signals, and runs long-lived coordination workflows. Both ship in a single container behind a single HTTP port.

**Assignment theme: Theme 3 — Systems & Reliability.**

---

# Problem Statement

## Prologue

The original motivation for this project comes from running large scale cloud infrastructure where foundational elements (ex: compute, networking etc) have their own unique set of data stores (ex: logs, state) which are inaccessible to their peer elements (ex: OVN Central is visible only to Networking On-calls, while VAST Configuration is visible only to Storage On-calls).

In this system, On-calls that are focused on a specific element (ex: Storage) are able to query their agents for information based on some behavioral patterns they are noticing (ex: Unable to mount a volume). They communicate with their agents through Slack sharing important context such as project id, VPC id, etc. Asynchronously, all subsystems also query their backend data stores (ex: OVN Central by Networking) to obtain information and store it in a normalized format for querying (ex: JSON).

When a Storage Agent requests information, the Arbiter (the master orchestrator) coordinates the request and response between the two agents working with an LLM. The output includes a summary of the root-cause and commands that need to be executed by one or more of the On-calls to resolve the situation (ex: the VM did not have the right interface IP address that storage was expecting. The resolution is to either update the VM's interface IP address or the binding in storage, clearly stating why one is recommended over the other).

## Scope

The Cloud Infrastructure Agent use-case is a specific implementation involving various orchestration, compute, storage and networking related concepts. To translate it into a more general use-case, we substitute a common concept that is widely used and understood — Movie Nights and Events.

Friends and families enjoy spending time together watching their favorite TV shows or movies. Each one in a group has their own preferences that determine the content they enjoy. For a movie night or a date night to work, these preferences need to match. Friends and families also enjoy movie events that match their tastes (ex: the 25th year anniversary of Lord of the Rings), movie marathons (ex: Star Wars) and seasonal movies (ex: scary movies every weekend in October). Content is also widely distributed with various streaming services offering these movies at different times for different financial considerations (subscriptions, rentals, free with ads etc.)

A system that understands the tastes of each member, that can communicate with other entities in the group and identify common patterns, and that sets up events while continuously looking for suitable options, will make family movie nights more enjoyable.

## Why the analogy holds

The property that makes the cloud case hard is not the domain. It is that **each participant holds context its peers cannot read, and a correct answer requires reasoning across all of it.**

Movie nights preserve that property exactly:

- Each member's taste profile is private and held by their own agent.
- A group decision is only correct if it accounts for all members.
- No member should learn what another member privately wants to watch — the socially interesting failure mode is not a crash, it is a leak.

Substituting the domain removes the reviewer's need for domain expertise — an explicit assignment requirement — while keeping the hard part intact.

---

# Theme and Thesis

**Theme 3 — Systems & Reliability.** The theme offers three paths. Agora claims two of them: *a tool that solves a real workflow pain point*, and *infrastructure that degrades predictably under stress*.

## The real workflow, and where it hurts

The workflow described in the Prologue is cross-team incident diagnosis. It is slow for reasons that have very little to do with how hard the diagnosis actually is. Three pain points, all of which Agora addresses directly.

Two of the three are **inherited** from the cloud domain. One is **net-new**, introduced by the translation into the family domain — and that asymmetry is itself part of the argument, so it is called out rather than smoothed over.

### Pain 1 — The data required for a correct answer cannot be pooled

**Today:** the Storage on-call cannot read OVN Central. Not "should not" — *cannot*. Access is gated by team boundary, compliance scope and blast-radius policy. So diagnosis proceeds as a serialized human relay in Slack: ask, wait for a peer who is asleep or already paged, receive a partial answer, ask a follow-up, wait again. **MTTR is dominated by cross-boundary round-trips, not by the difficulty of the root cause.**

**Why the obvious fix is blocked:** the naive AI answer — put every team's logs in one context window and let the model reason over all of it — is precisely what the boundary exists to prevent. The pain is real *and* the obvious solution is prohibited. That combination is what makes it worth building for.

**What Agora does:** the arbiter reconciles sealed, derived signals from each participant. The raw context is never pooled, so no boundary is crossed, and the round-trips happen in parallel at machine speed instead of serially at human speed.

### Pain 2 — Attribution. Net-new, introduced by the domain translation

This pain is **not inherited from the cloud case**, and the distinction is worth being precise about rather than overstating for narrative tidiness.

In cloud infrastructure, attribution is largely acceptable. It is fine — often desirable — for Storage to know how Networking is configured, and for a root-cause summary to name which subsystem held the wrong binding. The participants are *teams separated by access control*; the boundary is about reachability, not exposure. **Anonymity is not a critical ask there.**

It becomes one the moment the domain is translated, because the participants stop being teams and become **people with ongoing relationships to each other**. If the family slate says *"we picked this because Priya wanted something short,"* Priya is teased, negotiated against, or simply stops telling her agent the truth. The same output that is useful between subsystems is corrosive between siblings.

So the target domain is **strictly harder than the source domain along this one axis**. Porting a systems problem into a human one does not merely relabel the constraints — it adds one. That is a more interesting claim than pretending the pain was there all along.

**What Agora does:** a constraint enters the joint explanation only if at least *k* participants share it, and it is never attributed to a source.

**Consequently *k* is the domain-translation knob, not a magic number.** `k = 1` reproduces the cloud case, where attribution is acceptable and every constraint is speakable. `k = 2` is the family setting used here. `k = n` is total anonymity. One parameter spans both domains, which means the prototype implements a **superset** of the source domain's requirements and could be ported back by changing a single value.

### Pain 3 — Coordination across a boundary is where the system hangs

**Today:** diagnosis blocks on whichever participant does not answer. A coordinator that waits for everyone is a coordinator that waits forever — at 3am, during the incident it was supposed to shorten.

**What Agora does:** parallel fan-out under a per-participant deadline, with the decision proceeding at reduced quorum and labeled as such.

## How the three pains map across domains

Family movie night is not a toy restatement — but it is not a straight copy either.

| Pain | Cloud infrastructure | Family movie night | Status |
|---|---|---|---|
| **1. Cannot pool** | Hard boundary — Storage *cannot* read OVN Central | Members' taste profiles are private and there is no shared store to pool them into | **Inherited**, load-bearing in both |
| **2. Attribution** | Largely a non-issue. Naming which subsystem held the wrong binding is acceptable and often useful | Naming who wanted what causes teasing, negotiation, and members going quiet with their own agent | **Net-new** — appears only after translation |
| **3. Hanging coordination** | Diagnosis blocks on the peer who is asleep or already paged | A convene blocks on the member still at work | **Inherited**, load-bearing in both |

The prototype therefore satisfies the source domain's requirements *and one more*. Setting `k = 1` collapses it back to the cloud case exactly.

## Thesis

**The non-obvious claim:** a group decision can be produced from private contexts that are *never pooled*, and the arbiter can justify that decision **without revealing who wanted what**.

Two distinct properties, in increasing order of difficulty:

- **Isolation** — no member can read another member's raw context. Necessary, relatively easy, and **inherited from the source domain**.
- **Anonymity** — no member can *attribute* a derived signal to a specific other member. Much harder, and **net-new to the target domain**. A family is its adversarial case: in a five-person group, "someone wants something short" narrows quickly; in a two-person group it is fully re-identifiable. Choosing a single small family *raises* the difficulty of this claim rather than lowering it.

Isolation and anonymity are enforced at three separate layers, because each defeats a different attack: content de-identification at signal emission defeats verbatim and paraphrase leaks; identity stripping at the fan-out boundary defeats direct attribution; and the *k*-threshold at reconciliation defeats **inference from rarity** — the "only one person could possibly want that" attack, which stripping names does nothing to prevent. Only the third layer uses *k*.

The design insight is that these properties and reliability are the **same mechanism**. Member agents emit only sealed, derived signals rather than raw context. Because the arbiter never needs any member's full state, it can proceed correctly when a member times out. Most systems trade isolation against availability; here isolation is what makes graceful degradation cheap.

This is also the sharpest answer to *"what could a single LLM call with everyone's preferences in the prompt not do?"* Such a call could produce a comparable slate. What it could not do is **obtain the inputs**. In both domains, pooling is what blocks it: the context cannot legally or practically be assembled in one place. In the family domain a second block applies — people share honestly only with something that will not attribute their position back to them.

Systems properties the prototype must demonstrate:

- **Concurrency** — all members of the group are consulted in parallel within a single workflow, as real goroutines under a per-member deadline, not sequential calls.
- **Partial failure** — a member agent that times out or errors does not fail the workflow. The decision proceeds at reduced quorum and is explicitly labeled provisional, naming how many members were represented.
- **Predictable degradation** — if the LLM is unavailable, slow, or returns unparseable output, the arbiter falls back to deterministic scoring over the local catalog and says so in the response. The system never fabricates a recommendation and never hangs.
- **Durable long-running workflow** — convene state is persisted before any external call. A workflow survives process restart and is resumable from its last checkpoint.

---

# Goals

1. **Conversational agent.** Users chat with their agent to share preferences, request recommendations, and receive pending notifications.

2. **Private per-user context.** A user's raw preference text and derived profile are readable only by that user's own agent session. No path returns one user's raw context to another user.

3. **Anonymised group reconciliation.** The arbiter fans out to member agents in parallel, receives sealed derived signals, and reconciles them into a ranked group slate. A constraint may appear in the justification only if at least *k* members share it; below that threshold it still influences ranking, silently.

4. **Durable, fault-tolerant convene workflow.** Checkpointed before external calls, resumable after restart, correct under partial member failure and under LLM outage.

5. **Seeded catalog.** A bundled, normalized catalog of titles, genres, availability windows and dated events, queried by the LLM through tools rather than fetched at request time.

6. **Zero-install deployed access.** A public URL reachable by `curl` with no local installation, code execution, or compilation — for the prototype *or* for the client. No web UI: raw JSON is both sufficient and, for the isolation claim specifically, more credible than a rendered page a reviewer would have to trust not to be filtering.

7. **Reviewer-legible demo.** Seed data and demo controls sufficient to exercise every user story in under five minutes with no reviewer-supplied inputs.

---

# Non-Goals

Explicitly out of scope for the prototype. Each is a known-solved problem, documented as a path-to-production item rather than implemented.

1. **Authentication, authorization and identity.** Users are identified by an opaque `user_id` supplied in the request.
2. **Collector service and live scraping** (TMDB, JustWatch). Replaced by a bundled catalog snapshot. Live collection writes into the same normalized schema in production, so this is an additive change.
3. **gRPC and protobuf transport.** Agent and arbiter are Go packages in one binary, separated by an arbiter-owned interface. The `.proto` service definition is written as a design artifact but not wired; the production split is a second implementation of an existing interface.
4. **Group creation, invitation and join semantics.** The prototype ships one fixed family group.
5. **Multi-group membership and cross-group isolation.** Within-group anonymity is the harder and more interesting case and is retained.
6. **Notification transport.** No email, Slack, or push. Notifications are piggybacked onto the next response.
7. **Horizontal scale, HA, clustered storage, Kubernetes, service mesh, GitOps.**
8. **Token and cost optimization.**
9. **Any web UI at all.** Cut deliberately, not for time alone. The assignment permits an API-only submission, the required ~5 minute video already carries the visual legibility a page would have provided, and for an isolation claim a rendered view is *less* credible than raw JSON — a page is a layer that could be filtering client-side. A UI would also add a second test surface and an independent failure mode for no distinct gain. Recovered instead by the `walkthrough` endpoint, which shares its implementation with the demo smoke test.

**Note the split from Assumption 2 below:** authentication is out of scope, but **cross-user isolation and anonymity are in scope** and are graded properties of the prototype. These are different things and were previously conflated.

---

# Assumptions

1. Implementation decisions are optimized for build time and portability. Total build budget is under 8 hours, with the prototype runnable locally for development and deployed publicly for review.

2. **Callers are trusted as to identity, not as to content.** We assume a caller is the `user_id` they claim. We do *not* assume they will refrain from attempting to extract or attribute another member's context — no user's raw context is surfaced to another user, and no derived signal below the *k* threshold is attributed, regardless of how the request is phrased.

3. LLM access uses a server-side API key. Reviewers never supply credentials.

4. The demo runs against a bundled catalog snapshot with a simulated clock, so behavior is deterministic and reproducible regardless of when it is opened.

5. One fixed family group of roughly five members exists at seed time. Members do not join or leave during a session.

---

# User Stories and Acceptance Criteria

**US1 — Share preferences.**
*As a member, I share my tastes with my agent in natural language.*
Given a seeded member, when they send free-text preferences, then the agent acknowledges specifically (not generically), and the stated preference is retrievable in that member's subsequent session context.

**US2 — Immediate recommendation.**
*As a member, I ask my agent for something to watch right now.*
Given a member with stored context, when they request a recommendation, then the response contains titles from the seeded catalog with availability, and a one-line justification referencing that member's own stated preferences.

**US3 — Group convene.**
*As a member, I ask for a movie night that works for the family.*
Given the seeded group with deliberately conflicting tastes, when a member requests a convene, then the arbiter consults all members in parallel and returns a ranked slate with a justification referencing group-level constraints.

**US4 — Isolation and anonymity (headline).**
*No member learns, or can attribute, another member's private context.*

- *Isolation:* given member A holds a preference B has not shared, when B convenes the group or directly asks their agent what A wants, then no response to B contains A's raw preference text or a paraphrase attributable to A.
- *Anonymity:* when a constraint is shared by fewer than *k* members, then it does not appear in any justification shown to the group, though it still affects ranking. Justifications cite only constraints meeting the threshold, and never name a member as the source.

**Testable assertions, enforced as build gates:**
1. Every seeded member's private preference strings are matched against every response returned to any other member; zero matches.
2. No justification string contains a member name or a below-threshold constraint. Includes LLM-generated text, which is the likeliest accidental leak path.

**US5 — Predictable degradation.**
*The system stays correct when parts of it fail.*
Given an injected member-agent timeout, when a convene runs, then the workflow completes, the result is marked provisional, and the response states how many of N members were represented. Given an injected LLM outage, when any request is made, then the arbiter returns a deterministically-scored result labeled as degraded, rather than an error or a fabrication.

**US6 — Time-shifted workflow and notifications.**
*Long-running coordination is observable in a five-minute review.*
Given a scheduled convene or a seasonal event window, when the simulated clock is advanced, then the pending workflow progresses and the resulting notification is delivered on the affected members' next response.

---

# Self-Contained Evaluation and Demo Mode

This section exists because the assignment names self-contained evaluation as a critical requirement. Demo data is a product requirement, not a testing convenience.

**Seed state (bundled, no reviewer input required):**
- One family group of ~5 members with deliberately conflicting tastes, including at least one preference held by exactly one member — the case the anonymity threshold must suppress.
- A catalog snapshot of a few hundred titles with genre, era, runtime, and availability windows across several services.
- A small set of dated events (anniversary re-releases, seasonal windows) positioned relative to the simulated clock.

**Demo controls:**
- `reset` — restore seed state.
- `tick` — advance the simulated clock so scheduled workflows and event notifications fire on demand (satisfies US6).
- `chaos` — inject member-agent timeout or LLM outage (satisfies US5).
- `walkthrough` — run the entire scripted scenario server-side and return a **narrated transcript**: each step with its actor, request, response, and the assertion it demonstrates. One `curl`, all six user stories, including the comparative view of the same convene as seen by different members.

A reviewer with no prior knowledge must reach every user story through these controls. `walkthrough` is the zero-effort path; the individual endpoints are there for anyone who wants to poke at it themselves.

---

# Deliverables

1. **Functioning deployed prototype** at a public URL, exercised entirely via `curl`. No local install of anything, including client tooling. A `README` with copy-pasteable request blocks and a `demo.sh` remove the friction of hand-constructing requests.
2. **Source code** in a public GitHub repository.
3. **Design rationale**, in both formats: a written document (`ClaudeSummary.md`) and a ~5 minute recorded video. Both must cover: why this theme and approach, what is non-obvious about the idea, key design decisions and tradeoffs, how it would be extended with more time, and approximately how long it took.
4. **AI transcripts**, alongside the code (`ClaudeFeedback.md` plus session exports).

---

# Success Criteria

The prototype succeeds if a reviewer with no context and no data, within five minutes, can:

1. Hold a conversation with a seeded member's agent and see their preference retained.
2. Convene the family and receive a justified slate.
3. Attempt to extract *and* attribute another member's preference, and observe that neither succeeds.
4. Inject a failure and observe the system degrade with an honest, labeled answer instead of erroring or fabricating.
5. Advance the clock and observe a long-running workflow complete.

And can then state in one sentence what Agora does that a single LLM call with everyone's preferences in the prompt could not.

---

# Open Risks

- **Theme 3 in a 2–4 hour budget.** Mitigated by making isolation, anonymity and degradation the same mechanism, so one build satisfies all three, and by cutting transport and deployment ceremony rather than reliability behavior.
- **Anonymity is easy to claim and easy to violate accidentally**, especially through LLM-generated justification text. Mitigated by making both US4 assertions build gates rather than manual inspections.
- **The *k* threshold degrades explanation quality.** Suppressing minority constraints makes justifications vaguer. Mitigated by keeping *k* low (2) and by having suppressed constraints still influence ranking, so the *slate* remains correct even when the *explanation* is coarse. This tradeoff is worth stating explicitly in the rationale.
- **Convene latency.** Parallel fan-out plus an LLM loop may exceed a comfortable interactive response time — which US5 already requires the system to handle.
