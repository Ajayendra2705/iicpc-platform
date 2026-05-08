# syntax=docker/dockerfile:1.7
# Sandbox base image for Go contestant submissions.
# Convention: contestant source has go.mod at root.

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/main ./...

FROM gcr.io/distroless/static-debian12:nonroot
ARG ENTRYPOINT_PATH=/app/main
ARG RUNTIME_PORT=9100
COPY --from=build /out/main ${ENTRYPOINT_PATH}
USER nonroot:nonroot
ENV RUNTIME_PORT=${RUNTIME_PORT}
EXPOSE ${RUNTIME_PORT}
ENTRYPOINT ["/app/main"]
