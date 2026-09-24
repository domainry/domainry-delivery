# syntax=docker/dockerfile:1.7

ARG BASE_IMAGE_REGISTRY=

FROM ${BASE_IMAGE_REGISTRY}golang:1.26-alpine AS builder

WORKDIR /src
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./
RUN --mount=type=secret,id=github_token \
    if [ -f /run/secrets/github_token ]; then \
      git config --global url."https://$(cat /run/secrets/github_token):x-oauth-basic@github.com/".insteadOf "https://github.com/"; \
    fi && \
    go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY module ./module
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/domainry-delivery ./cmd/domainry-delivery

FROM ${BASE_IMAGE_REGISTRY}alpine:latest

RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 domainry && adduser -S -D -H -u 10001 -G domainry domainry

WORKDIR /app
COPY --from=builder /out/domainry-delivery /app/domainry-delivery

ENV DELIVERY_ADDR=:8096
USER 10001:10001
EXPOSE 8096
ENTRYPOINT ["/app/domainry-delivery"]
