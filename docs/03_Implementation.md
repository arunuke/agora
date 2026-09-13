# TL;DR

# Service Architecture

## Guidelines

- **Go as the primary language for all components** Go is strongly typed, has a native concurrency model, does garbage collection, has available gRPC, NATS and REST libraries.

- **JSON as the standard data exchange format** JSON is human readable and widely supported.

- **REST APIs for all external communication** Curl is available on most systems and is the only requirement for clients. Using grpcurl could have unified a singular interface for external and internal communication, but not considered since it adds additional complexity to the client (ex: usage of protobuf definitions and explicit service method calls).

- **Kubernetes based Orchestration for long-running services** Our services are long-running, need to be orchestrated and will be ready for scaling, reliabilty and canary deployments.

- **gRPC for Inter-service communication** NATS provides us event-based notifications, but gRPC and response piggy-backing allows us to demonstrate these features given time constraints.

- **SQLite and SQLite-Vec for state management** SQLite is a lightweight, file-based database that is easy to set up and manage. SQLite-Vec is a vector database that is used for storing and retrieving vector embeddings.

- **Managed Kubernetes Service** We will use a managed Kubernetes service like EKS or GKE for deployment.

## Agent 

User Agent is a long-running service that is deployed on Kubernetes implementing Go's concurrency model for handling multiple user requests simultaneously. It implements a REST API for external communication and gRPC for inter-service communication.

## Collector 

Collector takes a fixed set of input sites (TMDB and JustWatch for MVP) and scrapes them for movie data. It then sends a request to Arbiter which will store the normalized data in a SQLite database. By keeping the list of input sites fixed, the prototype avoids the need to export an API for Collector.

## Arbiter 

Arbiter is the central service that manages the state of the system. It is responsible for storing and retrieving data from the SQLite database and managing the workflow of the system.

It exports the following APIs:

1. gRPC API for Agent to call with requests from clients.
2. gRPC API for Collector to call with movie data.
3. Tools API for LLM to call for retrieving movie data.

# APIs

## Restful APIs

## GRPC APIs

# Test Infrastructure

Generate unit tests for all services ensuring there is good code coverage (~80% aspirational)

Generate integration tests for all services where they are all run as containers on the development environment.

On 'make data' command, all the relevant data stores will be updated with test data for different users, groups, movies and shows. This will help us to rapidly test the system without having to manually create the data.

On 'make test' command, all the relevant tests will be run to ensure the system is working as expected.

# Artifacts

** K8s Manifests ** K8s manifests to deploy the Agent service and the Orchestrator service. These would be deployed onto a k8s cluster via kubectl.

** Docker Images ** Docker images for the Agent service and the Orchestrator service. These would be pushed to a container registry and pulled by the k8s cluster.

** Source Code ** Source code for the Agent service and the Orchestrator service. This would be available on a version control system like GitHub.

# Folder Structure

Create a folder structure for the project that is easy to navigate and understand.

Add all artifacts under the 'src' folder.

Create a 'services' folder under 'src' to store all service related code.

Create a 'deploy' folder under 'src' to store all deployment related code.

Create a 'test' folder under 'src' to store all test related code.

Create a common makefile that will be used to build, test and deploy the system. It will have targets to build all sources, run all tests, and deploy the system on the remote environment via kubectl.


