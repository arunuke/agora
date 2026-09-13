# Single image. The only binary artifact the prototype produces.
#
# Build and runtime stages share a Debian bookworm base so the linked libc
# matches — sqlite-vec is a CGO loadable extension, and this is where a
# mismatched base bites.

FROM golang:1.24-bookworm AS build

# sqlite-vec-go-bindings ships sqlite-vec.c and sqlite-vec.h but NOT sqlite3.h —
# its header does `#include "sqlite3.h"` and expects the system to provide it.
# golang:*-bookworm does not, so without this the build dies with:
#   ./sqlite-vec.h:7:10: fatal error: sqlite3.h: No such file or directory
#
# Only the BUILD stage needs it. mattn/go-sqlite3 compiles its own SQLite
# amalgamation, so the resulting binary has no runtime libsqlite3 dependency
# (verified with ldd) and the runtime stage stays slim.
RUN apt-get update \
 && apt-get install -y --no-install-recommends libsqlite3-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=1
RUN go build -o /out/agora ./cmd/agora

FROM debian:bookworm-slim
# curl for the compose healthcheck and for poking at the API from inside the
# container; jq so the bundled external client can ASSERT rather than just
# print. Together they make the image self-verifying on a remote host that has
# no tooling of its own:
#
#   docker exec agora bash /app/demo.sh http://localhost:8080
#
# That run exercises the app without touching the network path. If it passes
# and the same client fails from your desktop, the fault is the security group,
# not the application — which is the whole reason to run it in that order.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl jq \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/agora /app/agora
COPY seed /app/seed
COPY demo.sh /app/demo.sh
RUN chmod +x /app/demo.sh

# One process. Agent and arbiter are packages in this binary, not services, so
# there is nothing to supervise — no supervisord, no s6. The boundary that
# matters is the arbiter-owned MemberAgent interface, not a process boundary.
VOLUME ["/data"]
EXPOSE 8080
ENV AGORA_DB=/data/agora.db AGORA_SEED=/app/seed AGORA_ADDR=:8080
ENTRYPOINT ["/app/agora"]
