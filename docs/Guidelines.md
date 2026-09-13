# Context
As AI capabilities advance rapidly, engineers face a unique challenge:
building tools and experiences that are useful, trustworthy, and delightful. The best
products demonstrate taste, handle complexity gracefully, and solve real problems
in non-obvious ways.
This assignment asks you to build something that showcases your judgment,
creativity, and execution.
Time Expectation
Target: 1-2 hours. Hard limit: 8 hours.
This assignment is designed to replace a synchronous interview session. We expect
most candidates to complete something compelling in 1-2 hours, and we value your
time - something insightful and deeper is more valuable than breadth. You may
spend up to 8 hours if you're enjoying the problem, but this is not expected or
required—we've seen excellent submissions completed in under 2 hours.
If you find yourself exceeding 8 hours, stop and submit what you have. Part of what
we're evaluating is your ability to scope effectively.
In your written rationale, please note approximately how long you spent.
Objective
Build a functional prototype that demonstrates your ability to ship an impressive,
self-contained experience. Choose one of the following themes:
Theme 1: Exploration & Understanding
Complex systems, technical concepts, and unfamiliar artifacts are hard to
understand through static explanation. Build a tool that helps users develop deep

understanding—whether that's a simulation of emergent dynamics, an explainer for
a technical concept, or a tool for exploring codebases, datasets, or documents.

Theme 2: Creative & Generative Tools
Creative tools can unlock workflows that weren't previously possible. Build a tool
that gives users meaningful creative leverage—not just "generate" buttons, but real
control over iteration, variation, and refinement.

Theme 3: Systems & Reliability
Real systems must handle failure, concurrency, and scale. Build something that
demonstrates thoughtful systems engineering—whether that's a developer tool that
solves a real workflow pain point, a distributed primitive that handles failure
gracefully, or infrastructure that degrades predictably under stress.

Theme 4: Evaluation & Data Quality
ML research depends on rigorous evaluation and clean data—but both are
surprisingly hard. Build a tool or pipeline that addresses a real challenge in evals,
reward modeling, data quality, or experiment tracking.

Theme 5: Performance Optimization
Performance engineering requires understanding hardware, algorithms, and
tradeoffs. Build the fastest implementation you can of a meaningful component,
with benchmarks demonstrating your results.
This theme is evaluated primarily on actual measured performance relative to
baselines. Include benchmarks, methodology, and comparison to naive/reference
implementations.

Critical Requirement: Self-Contained Evaluation
Your prototype must be evaluatable without requiring specific data, documents, or
domain expertise from the reviewer.
If your concept requires domain-specific inputs, either bundle compelling examples
or provide a "demo mode" that showcases the tool's capabilities without requiring
the reviewer to source their own data.

Requirements
Create three deliverables:
1. A functioning prototype (deployed)
● The Anthropic team should be able to use and interact with the prototype
immediately in a browser or via an API.
○ Unfortunately we cannot accept submissions that require local
installation, code execution, or compilation.
● Feel free to focus on a single feature or interaction pattern
● Polish is less important than demonstrating your core idea effectively
● Must include sample data or demo mode if the tool requires specific inputs
2. Your code
● Submit via GitHub repo link
● Code quality matters but is not the primary evaluation criterion
● Use any languages/technologies you're comfortable with
3. Design rationale
● Use both formats: self-recorded video (~5 min) and a short written doc
● Why you chose this theme and this specific approach
● What makes your idea interesting or non-obvious
● Key design decisions and tradeoffs you made

● How you'd extend this with more time
● Approximately how long you spent

Use of AI During Development
You are expected to use Claude Code, Claude.ai, or other AI tools during
development. We want to see how you leverage these tools effectively—this is a
core skill for engineers at Anthropic. You are permitted to use competitor models if
you’d like.
Please submit your Claude/AI transcripts alongside your code. We're evaluating
your judgment: how you direct the AI, evaluate its outputs, make tradeoffs, and
maintain your own vision. A fully AI-generated project with no judgment will not
pass.