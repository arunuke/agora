# Single image. The only binary artifact the prototype produces.
#
# Build and runtime stages share a Debian bookworm base so the linked libc
# matches — sqlite-vec is a CGO loadable extension, and this is where a
# mismatched base bites.

FROM golang:1.24-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=1
RUN go build -o /out/agora ./cmd/agora

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/agora /app/agora
COPY seed /app/seed

# One process. Agent and arbiter are packages in this binary, not services, so
# there is nothing to supervise — no supervisord, no s6. The boundary that
# matters is the arbiter-owned MemberAgent interface, not a process boundary.
VOLUME ["/data"]
EXPOSE 8080
ENV AGORA_DB=/data/agora.db AGORA_SEED=/app/seed AGORA_ADDR=:8080
ENTRYPOINT ["/app/agora"]
