# TL; DR

This project builds a group of co-ordinated agents that work together to solve a specific problem. It implements an agent service that builds context from client interactions, a collector service that collects data from various sources, and an arbiter service that coordinates the agents and the associated infrastructure required to run it.

# Problem Statement

## Prologue

The original motivation for this project comes from running large scale cloud infrastructure where foundational elements (ex: compute, networking etc) have their own unique set of data stores (ex: logs, state) which are inaccessible to their peer elements (ex: OVN Central is visible only to Networking On-calls, while VAST Configuration is visible only to Storage On-calls). 

In this system, On-calls that are focused on a specific element (ex: Storage) are able to query their agents for information based on some behavioral patterns they are noticing (ex: Unable to mount a volume). They communicate with their agents through Slack sharing important context such as project id, VPC id, etc. Asynchronously, all subsystems also query their backend data stores (ex: OVN Central by Networking) to obtain information and stores it in a normalized format for querying (ex: JSON).

 When a Storage Agent requests information, the Arbiter (the master orchestrator) coordinates the request and response between the two agents working with an LLM. The output includes a summary of the root-cause and commands that need to be executed by one or more of the On-calls to resolve the situation (ex: the VM did not have the right interface IP address that storage was expecting. The resolution is to either update the VM's interface IP address or the binding in storage clearly stating why one is recommended over the other).

## Scope

The Cloud Infrastructure Agent use-case is a specific implementation involving various orchestration, compute, storage and networking related concepts. To translate it into a more general use-case, we will substitute a common concept that is widely used and understood - Movie Nights and Events.

Friends and families enjoy spending time together watching their favorite TV shows or movies. Each one in a group has their own preferences that determine the content they enjoy. For a movie night or a date night to work, these preferences need to match. Friends and families also enjoy movie events that match their tastes (ex: the 25th year anniversary of Lord of the Rings), movie marathons (ex: Star Wars) and seasonal movies (ex: scary movies every weekend in October). Content is also widely distributed with various streaming services offering these movies at different times for different financial considerations (subscriptions, rentals, free with ads etc.)

A system that understands the tastes of each member, that can communicate with other entites in the group and identifies common patterns, and sets up events while continuously looking for suitable options will make family movie nights more enjoyable.

# Goals

1. Build a user agent that users can chat with to share their preferences, query for recommendations and receive notifications of events.

2. Build a collector that collects data from various preferred sources and normalizes it for querying.

3. Build an arbiter that is responsible for communicating with LLMs and coordinating long running workflows involving coordinated responses from agents to create events and notifications.

4. Deploy this in a portable, external facing environment for easy access by remote users.

# Assumptions

1. A number of implementations decisions are optimized for time and portability. The entire project is expected to be built in < 8hrs of time with the option to be run locally (for development and testing) and in a cloud environment (that external users can access).

2. User authentication and security are not in scope for this project. It is assumed that the users are secure and trusted.

# User Stories

1. Users should be able to chat with their agent, share their preferences, and request immediate recommendations.
2. Users should be able to query their agent to receive periodic recommendations based on their overall usage patterns.
3. Users should be able to query for matching events with one or more users in their peer group.
4. Users should be able to receive notifications of approaching events that match their preferences.
