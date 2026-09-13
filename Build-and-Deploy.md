# Build

The primary objective is create a full-fledged pipleine via build targets that can be completed on the developer's local box. This speeds up the development cycle with strong iterative properties. The project is built using a single makefile that solves a variety of usecases. Each target builds on a previous target.

**build**: builds all the required go binaries needed
**package**: builds an installable. In this case, it is a docker image that can be pulled and run on any environment. This will also push the image to dockerhub.
**seed**: builds the pacakge and loads it with mock-data
**test**: run all unit tests
**pipeline**: runs the full pipeline that includes integration tests as well

# Test

The primary objective of test is to make sure that developers can fully validate all changes locally before pushing it to production. 

Testing is primarily split into two modes. The first mode is for unit testing and expects code coverage to be at least 80% (covered under `test` above). The second mode is pipeline testing which brings up the image locally, sets up a client that will test various scenarios using `curl` like an external user.

Test cases are built around the current scope and not for the North Star or Original Scope. This ensures we have a working version that meets all the functional requirements while also allowing for us to build additional test cases and validation as the system expands.

# Deploy

On a test machine, deployment is done by a docker-compose command that runs all the binaries. The eventual Target is a simple AWS EC2 server which will be prepared to run a docker image. The deployment will be done via docker-compose so that running is simplified since the configuration will be version controlled via docker-compose.yml

```
docker compose up -d
```

# Test Scenarios

The following test scenarios represent the core functionality and will ensure that any new changes do not regress from this behavior. This use-case sits above the tests already built by Claude.

1. Users A, B, C, D and E individually provide their favorite scenarios with different degrees of specificity. User A specifically says what genre they prefer. User B indicates they have no preferences. User C inputs no context. User D only says what movies they like. User E indicates they would like to match with User A.

2. User B asks specifically for preferences of another user. The system indicates that it cannot do that.

3. User A asks to schedule a christmas movie marathon for the group and select some movies and their streaming/screening choices.

4. User E asks for movie preferences that fit their profile. The system should ask for their preferences instead of matching with another user that will allow leakage of data.



# Code Review

Once we are able to fully build and deploy locally, code review will be done on each source file that is not auto-generated (ex: the Go files). This exercise will trim down what is required and what is not.