# TL;DR

This document outlines the design achieve the MVP specified in the Requirements document. It also describes the path to production by extending the MVP features without any major changes.

# Prototype Design

## Optimizations and Tradeoffs

The prototype design is optimized for the following:

- **Speed of Development** - The prototype is designed to be built and deployed as quickly as possible. It makes assumptions on various dimensions such as security which can be added during the path to production. Web interfaces, bot integrations and other extensions are dropped in favor of simpler, more direct communication methods.
- **Minimal Dependencies on external clients** - External clients should be able to interact with the system with simple tooling with no additional installations.
- **All communication is synchronous** - All communication between components is synchronous to keep the design simple.
- **LLM does the heavy lifting** - The LLM will be responsible for most of the intelligence in the system, including understanding user requests, generating responses, and managing the conversation flow. Native Agora elements are responsible for state management, data collection, anonymity & isolation, long-running workflows and notifications.
- **Normalize data** - Data from external sources should be normalized to a common format before being stored in the database. This common format is then used by the LLM to generate responses reducing query times (over the LLM scraping the data at that time) and improving token cost.
- **Single data store** - All data is stored in a single database (SQLite) for simplicity in the Arbiter service. This also allows us to make all the other services stateless.
- **Keep Production in Mind** - The prototype should be designed in a way that it can be easily transitioned to a production environment.

## Infrastructure Setup

The prototype will run on a Kubernetes cluster with the following deployment model:

- **Agent Service** - Exports a public IP that clients can use to communicate with the agent via REST API.

- **Arbiter Service** - Runs in the background and handles long-running workflows and notifications, responds to agent requests, connects with the LLM, and manages state. The Arbiter service is also responsible for managing data via SQLite and vector.

- **Collector Service** - Runs in the background and collects data from external sources, normalizes it using the LLM via the Arbiter service which stores it in a database to be fed to the LLM.

## Communication Flows

**Clients communicate with agent via CURL commands.** - External users engage with their agent via CURL commands.

```
curl -X POST https://agent.agora.ai/v1/message \
  -H "Content-Type: application/json" \
  -d '{
    "message": "I like watching christmas themed movies during the holiday season",
    "user_id": "user123",
    "session_id": "session123"
  }'
```
**Responses are used to piggyback notifications.** - Each request will receive a response from the agent in the format. If there are any notifications pending for the customer, we will use this opportunity to add that to the response over a notification-based mechanism:
```
{
    "response": "That's a great choice! I will notify you with christmas themed suggestions before the season begins",
    "user_id": "user123",
    "notifications": [
        {
            "type": "science_fiction_themed_suggestions",
            "message": "Did you know the following movies are re-releasing for their anniversary:",
            "data": {
                "movies": [
                    "Interstellar",
                    "Aliens",
                    "BladeRunner"
                ]
            }
        }
    ]
}
```

**Asynchronous data collection** - Data will be collected from a set of fixed sources needs to happen in the background asynchronously by the Collector Service. It will be send to an LLM for normalization and storage in a database.

An example of raw and normalized data is shown below:

```
Raw Data:
{
    "title": "Interstellar",
    "year": 2014,
    "genre": "Science Fiction"
}

Normalized Data:
{
    "title": "Interstellar",
    "year": 2014,
    "genre": "Science Fiction",
    "normalized_title": "Interstellar",
    "normalized_year": 2014,
    "normalized_genre": "Science Fiction"
}
```

**Agents communicate with Arbiter via gRPC** Agents send requests received from clients to the Arbiter Service via gRPC and receive responses. 

**Arbiter communicates with LLM in a loop** The Arbiter Service will communicate with the LLM in a loop to process requests and generate responses. It uses two common patterns:
- **Tool Calling**: The LLM can call tools to retrieve data from the database or external sources.
- **RAG**: The LLM can use Retrieval-Augmented Generation to reason about the data and generate responses.

**Arbiter updates state and sends response** The Arbiter Service will update the state of the agent and send the response back to the agent.

## Storage

**Local file-based storage** For the prototype, data will be stored in local files. The file-backed datastore will be hosted alongside the Arbiter service.

**Relational and Context DAta** Use SQLlite and SQL Vec to store relational and context data in the same file-backed data store.

**Relational Schema** Use tables to identify users and sessions. The user table has user information with user id as the primary key while the session table is a normalized table that stores session information with session id as the primary key. Context related information will be stored in the SQL Vec database with context id as the primary key and session id as the foreign key.

## Deployment

**Kubectl based deployment** The prototype will be deployed using kubectl commands. In production, a more robust deployment strategy (ex: ArgoCD with Helm charts) will be used.

## API Design

Each GRPC API will follow a consistent pattern:
- Every method has a distinct Requests/Returns type defined in the protobuf file
- Every method has a protobuf-to-domain layer which looks for errors and strict enforcement of input parameters to fail-fast.
- Error types are well-defined.


### REST API

**Client Query API** - Clients can query the system for information using a simple REST API that implements a POST request with a JSON body containing the query.

### gRPC API

Indicative API names are given below.

**Agent Communication API** - Agents communicate with the Arbiter Service via gRPC.
- ProcessClientRequest(ClientRequest) -> ClientResponse
**Collector API** - Collector Service communicates with the Arbiter Service via gRPC.
- ProcessCollectorRequest(CollectorRequest) -> CollectorResponse
**Arbiter API** - Arbiter Service communicates with the LLM via gRPC.
- ProcessLLMRequest(LLMRequest) -> LLMResponse
This method should be an interface that can be implemented by different LLM providers.

## Arbiter Workflow

Since the Arbiter is the central element that (a) responds to agents (b) responds to collector (c) communicates with the LLM in a loop and (d) persists data, it is important to understand the workflow.

### Agent Related
- Receive a request from an agent with a JSON payload
- Collect information about the user's current context. If no current context available, use the last available context. If no context is available, let the LLM know.
- Receive various types of responses from the LLM to update the SQL database, the user's current context and a response to send back to the agent.

### Collector Related
- Receive a request from the collector with scraped information
- Send it to the LLM to normalize it and store it in the SQL database

### LLM Related

- Send the received request from the agent to the LLM

## Arbiter Tools
We would one or more tools within the Arbiter that directly contribute to each user story listed in the requirements.

1. User sends preferences only : update_user_context
2. User queries for immediate recommendations : get_recommendations_from_context
3. User queries for matching events with peers in their group : get_matching_events_in_group
4. User receives notifications : send_notification
5. User requests asynchronous updates : set_asynchronous_task

# Production Design

This section talks about how the prototype can be easily transitioned to a production environment.

- **Interfaces** External users will use their existing messaging interface (eg: Slack) with the agent running as a bot. For minimal clients, a web interface will be provided. These extensions can use the existing REST endpoints.

- **Notifications** Notifications will sent asynchronusly via the same interface the user has backed by a message bus like NATS.

- **Administrative Tooling** - Administrators would be provided tools to manage the list of data sources, the timing of the collection loops and the choice of LLMs to process the data. Since all APIs are available as REST or GRPC, this is easier to achieve.

- **NATS for asynchronous communication** - NATS will be used for asynchronous communication between services where notifications can be shared from one service to the other asynchronously via a message bus.

- **Clustered Storage for HA** - State will be stored in an HA DB cluster.

