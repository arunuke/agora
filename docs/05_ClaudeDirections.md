
# TL; DR

This document describes the initial action plan shared with Claude. We will review this section by section to ensure we are aligned on the approach. We will move to a second section only after we have successfully addressed all open questions in the current section.

# Product Requirements

Review the Requirements.md file and critically analyze product requirements to determine if they meet the goals of the project. Use the PlatformSWETakeHome assignment PDF as a reference for the goals of the project. Ensure every ask (ex: client access, themes etc.) is addressed.

# Design

Review the design to see what components can be combined or simplified to reduce complexity. Optimize for rapid development and fewer moving parts while achieveing the requirements laid out in Product Requirements above. 

Consider path to production where the prototype can be enhanced with additional features and scalability. For example, running a local, file-based database in the prototype can be replaced with a cloud-based database in production and its an acceptable trade-off for rapid development.

If a gap is a known solved problem (ex: JWT based authentication for REST APIs), document the solution but don't implement it in the prototype as a production enhancement. These can be solved given enough time.

## Diagrams

Build mermaid diagrams for the following:
- System architecture that describes the three services and their relationships with the external user on the front-end and the LLM on the backend
- A query-response use-case
- A workflow use-case

# Implementation

- Review the set of existing APIs and determine if they are adequate for the MVP
- Add additional details on Tool APIs as needed
- Verify the APIs are named as per preferred Go/GRPC patterns.
- Consider implementing the LLM calling APIs as interfaces that can be swapped out for different LLM providers

## Agent 

Start by implementing the agent service in Go. Use a popular web framework (Gin preferred, but if a simpler alternative is available, use that). It will implement the POST REST API for client queries.

The data store for the agent service will also reside in the Arbiter, so Agent will talk to Arbiter over a published gRPC API. The response to this API call will be sent to the client.

## Arbiter

This is the core of the prototype and will require the most development time. We will approach the tasks step by step by focusing on the Arbiter Workflow section in the Design.md file.

**Determine the workflow** - Review the Design.md file and determine the workflow for the Arbiter service. Critically analyze this to remove any scope creep so we can focus on the core functionality for the prototype.

**Determine the data model** - Review the Design.md file and determine the data model for the Arbiter service. The current design talks about storing data in SQLlite and SQL Vec databases with a simple user table, session table and context data with their respective primary keys and associated foreign keys. Critically analyze this design and suggest improvements that would simplify the system while keeping up with the core principles.

**Implement the incoming GRPC APIs** - Implement the gRPC APIs that will be used to communicate with the agents and collector services.

**Plan the LLM integration** - Determine how the LLM will be integrated into the Arbiter service by identifying the tools required based on the use-cases we support from the Requirements.md file.

**Implement the Tools** - Implement the tools that will be used by the LLM to interact with the system.

## Collector

Run the collector service to collect data from the web using available scrapers or APIs. If no such APIs are available, use the 'make test-data' command to generate mock data. The collector service can then be marked as a production time development task and de-scoped for the prototype.

# Feedback

Include notes of all feedback to and from Claude in a new markdown file called ClaudeFeedback.md. This will serve as the historical record for where we started, what decisions were made, and why.

Summarize key questions from these discussions and add them as questions/answers in the FAQ.md file.

Lastly, build a full document called ClaudeSummary.md that summarizes the entire project, including the design, implementation, and any feedback received into a single readable file.
