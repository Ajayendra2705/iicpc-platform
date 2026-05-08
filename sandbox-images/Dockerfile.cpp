# syntax=docker/dockerfile:1.7
# Sandbox base image for C++ contestant submissions.
# Convention: contestant source has Makefile or CMakeLists.txt at root that
# produces a binary at one of: ./main, build/main, bin/main.

FROM debian:bookworm-slim AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential clang cmake make pkg-config \
    libboost-all-dev libssl-dev zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
RUN if [ -f Makefile ]; then \
        make -j$(nproc); \
    elif [ -f CMakeLists.txt ]; then \
        cmake -B build -S . -DCMAKE_BUILD_TYPE=Release && cmake --build build -j$(nproc); \
    else \
        echo "no Makefile or CMakeLists.txt at archive root" >&2 && exit 1; \
    fi && \
    if   [ -x ./main ];      then cp ./main      /out-main; \
    elif [ -x build/main ];  then cp build/main  /out-main; \
    elif [ -x bin/main ];    then cp bin/main    /out-main; \
    else \
        echo "no executable at ./main, build/main, or bin/main" >&2 && exit 1; \
    fi

FROM gcr.io/distroless/cc-debian12:nonroot
ARG ENTRYPOINT_PATH=/app/main
ARG RUNTIME_PORT=9100
COPY --from=build /out-main ${ENTRYPOINT_PATH}
USER nonroot:nonroot
ENV RUNTIME_PORT=${RUNTIME_PORT}
EXPOSE ${RUNTIME_PORT}
ENTRYPOINT ["/app/main"]
