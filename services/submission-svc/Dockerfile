# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.work go.work
COPY services services
RUN go work sync
WORKDIR /src/services/submission-svc
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/submission-svc .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/submission-svc /submission-svc
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/submission-svc"]
