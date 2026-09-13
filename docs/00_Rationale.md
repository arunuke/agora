*Read this first — and know that it was written last.*

This document orients: why the project exists, which theme it answers, and what
was traded away to fit the time available. It is deliberately numbered `00`
because everything else assumes the context it establishes.

It is also retrospective. It describes the system as it ended up, so it already
knows that the collector was scrapped and the services collapsed into one
process — decisions that [02_Design.md](02_Design.md) and
[03_Implementation.md](03_Implementation.md) still describe in their original,
unconstrained form. Those documents are preserved as written; the delta between
them and what was built is [06_Rescoped.md](06_Rescoped.md).

---

# Theme

Of the various themes identifed in Guidelines.md, I chose **Systems & Reliability**.

Systems are becoming increasingly complex, but current day tooling allows developers to rapidly build, iterate and deploy services at much reduced timelines and people. I wanted to build a system that brings various components working together for an enjoyable experience to consumers. This project has its roots in a real world infrastructure problem, exhibits well-defined system characteristics and makes design decisions that demonstrate reliability.

## Real-World Problem

The foundation for this project from the real-world infrastructure problem of having different islands of data, personnel and security credentials brought together to manage the infrastructure as a whole fabric. On-calls and Production Engineers that are specialized in their respective domains should be able to use their limited credentials to work with an LLM that can analyze the data and respond with a solution that may span multiple domains.

This project takes this problem and translates it to a common concept that many can relate to - scheduling movie nights with the family. In this project, members of a family (or any social group) discuss their preferences with their agent. When they intend to schedule an event, they can work with an LLM to identify options that fit the social group's preferences at that time without revealing any user-identifiable information.


## System Characteristics

### Orchestrated Components

The project brings together a number of components and orchestrates them together to deliver the desired user experience. The build cycle starts with generating all the needed binaries and then embedding them into a docker image for portability. The core services built - agent and arbiter - participate in a workflow collecting information from the user, persisting in the data store and then engage with a third component - the LLM - to produce responses back to the user.

### Well-defined Boundaries

A system can be reliable only when each component has well-defined boundaries which describe what they can and cannot do. In our system, the agents are bounded by collecting information but do not persist data while the arbiter is responsible for persisting information and context and sending anonymized data to LLM. The LLM acts primarily as a workhorse based on the inputs it receives and generates a response and is not expected to persist context within itself.

### Developer Experience

A system can continue to grow only if developers have an easy experience to participate in the development of the system. This project is built with this in mind so developers with expertise in different areas (Go application develoment, infrastructure, LLM integration) can all continue to build within the same space, test and validate components locally and deploy securely to external systems.


## Reliability

**Fallbacks**

The system introduces a local LLM that can be consulted if the preferred LLM is not available at that time. This ensures that users continue to get reliable responses and the system gracefully degrades over a complete failure.

**Data Isolation and Anonymity**

Data from each user is stored in a persistent data store which is not viewable by other users or their agents. Data can be accessed only through the Arbiter component which anonymizes the data before sending it to the LLM so that one user cannot determine the preferences set by another user.

# Approach

## Scope

The first step in the project was to define the scope of the project with particular considerations to the amount of time involved. A number of tradeoffs in scope, design and implementation are all tied to time.

The initial scope involved deploying the services as close to a production deployment as possible - separate k8s services fronted by a load balancer in an EKS or similar environment. However, optimizing for time, we collapsed the different services into individual processes that run in a single container. This also allowed the entire service to be run just with public Go APIs instead of relying on gRPCs.

## Design

The design of the project had to reflect the goals set in the Theme section above. This required an agent layer (ie, a collection of agent instances serving one or more users) which co-ordinate with the arbiter layer by sharing inputs received from the users.

The arbiter layer communicates with the LLM to normalize the inputs and store them in the backend. It also exchanges anonymized information with the LLM and shares the generated responses with the users. The arbiter layer implements various tools that the LLM can use to read information from the databases and provide corresponding results.

The collector layer, which was expected to crawl the web and store release/streaming information, was completely scrapped and instead pre-canned data was used to simulate already crawled information.

The project allows the developers to choose an LLM backend (simulated, open or subscription based) to be run across various environments with the final deployed version always using a subscription based model.

## Implementation

The implementation layer of this was driven entirely with the assistance of Claude. REST was chosen as the preferred API layer for clients to communicate with since it enforces the least dependencies (only curl is required from the client).

All services were built using Go using publicly available frameworks (ex: net/http) when available. SQLite was chosen since it supports both relational and contextual data formats. Docker was chosen as the runtime engine due its widely available options (ex: docker desktop for local development, docker-compose for a simpler rollout process).


## Validation

Validation is a time-consuming, manual process which this projects automates for rapid development and rollout. Unit tests are added to be a light-weight check against any new code added. The project also provides a full local pipeline validation with a simulate or local model that can be quickly tested before deployment.

The validation process also includes an integration test that is built with scenarios that mirror the user stories defined in the scope. This allows the developer to ensure, using automated means, that the service still meets feature requirements and there are no regressions.

## Rollout

Rollout is also done via specific targets in the Makefile for simplification. For production environments, this would be done via a service similar to Argo.


# Design Decisions & Tradeoffs

## Time Optimized

The following design decisions were all made to address the time constraint.

**Deployment Model** - From a fully distributed system with independent failure domains to a single process running in a docker image

**Localized DB** - Using a filebacked SQL instance to persist relational data and context over using a distributed DB deployment or a DB service.

**Enhanced local development** - Provide build targets that will customize and deploy locally so developers can build individual layers with no additional dependencies. Testing is also done locally before the rollout to external hosted instances making the build/test/deploy pipeline efficient.

**Production Plans** - Make every design decision extensible to production. Internal Go methods can be plugged underneath gRPC interfaces to make the services truly distributed, DBs can be run in a cluster or a managed offering, LLM backends can be plugged in by swapping out API service keys, CLI-based clients can be integrated into any app and hosted services can be swapped out by changing access keys.

**Security** - The project assumes trusted clients and hence ACLs and other security related features such as TLS are minimally enforced. 

**LLM Heavy-Lifts** - The LLM is used for every other task besides the scope for agents (collect information from clients) and arbiter (persist information in database, exchange information with the LLM). The LLM assists not only in generating the responses, but also in normalizing data to be stored in the DB.

# Key Challenges

**LLM Integration** - The LLM integration was one the more complicated parts of this solution. In the actual production use-case for the infrastructure agents/arbiter, a home-grown Managed Inference service was used which provided native API keys. However, for this effort, I had to use a few different methods to achieve the desired effect. For local builds, I used a simulated model where an LLM was simulated. This also acts as the fallback when LLMs are not available. Local builds were also enhanced with an open-source llama model weight. Deployed instances that clients can use will use Anthropic.

**Anonymity**

Although Anonymity was not a use-case for the infrastructure problem, I wanted to add it here since user-groups do not have to be restricted to just families. Large public groups with unconnected users can throw surprise events for that group without necessarily revealing individual user preferences. However, implementing it and validating for leaks was non-deterministic based on the responses observed from the simulated/llama/anthropic versions. As an example, a llama based test just made up a non-existent claim about a user's preferences.

**Human Code Reviews**

Human code review was limited due to lack of time. Validation was purely  through automated tests that align with the use-cases and validated mostly sunny-side outcomes.

That trade is accounted for in [ClaudeFeedback.md](ClaudeFeedback.md) under *Method — tests instead of line-by-line review*, including the three defects a reader would plausibly have caught and the automated suite did not.