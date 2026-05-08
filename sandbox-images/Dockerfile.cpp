# syntax=docker/dockerfile:1.7
# Sandbox base image for C++ contestant submissions.
# Convention: contestant source has Makefile at root that produces ./main.
# Build dependencies pre-installed: g++, clang, cmake, make, pkg-config, boost.

FROM debian:bookworm-slim AS build
ARG ENTRYPOINT_PATH=/app/main
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential clang cmake make pkg-config \
    libboost-all-dev libssl-dev zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
RUN make -j$(nproc) && \
    test -x ./main && cp ./main /out-main

FROM gcr.io/distroless/cc-debian12:nonroot
ARG ENTRYPOINT_PATH=/app/main
COPY --from=build /out-main ${ENTRYPOINT_PATH}
USER nonroot:nonroot
EXPOSE 9100
ENV ENTRYPOINT_PATH=${ENTRYPOINT_PATH}
ENTRYPOINT ["/app/main"]
