FROM golang:1.26-bookworm AS build
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ENV CGO_ENABLED=1
RUN go build -o /out/bot ./cmd/bot
RUN go build -o /out/fetcher ./cmd/fetcher

FROM debian:bookworm-slim
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/bot /app/bot
COPY --from=build /out/fetcher /app/fetcher

VOLUME ["/app/data"]
