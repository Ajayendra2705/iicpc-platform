# syntax=docker/dockerfile:1.7
# Sandbox base image for Go contestant submissions.
# Convention: contestant source has go.mod at root.
# Output binary path is configurable via ENTRYPOINT_PATH build arg.

FROM golang:1.22-alpine AS build
ARG ENTRYPOINT_PATH=/app/main
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/main ./...

FROM gcr.io/distroless/static-debian12:nonroot
ARG ENTRYPOINT_PATH=/app/main
COPY --from=build /out/main ${ENTRYPOINT_PATH}
USER nonroot:nonroot
EXPOSE 9100
ENV ENTRYPOINT_PATH=${ENTRYPOINT_PATH}
ENTRYPOINT ["/app/main"]
