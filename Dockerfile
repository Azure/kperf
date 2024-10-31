FROM golang:1.22 AS build-stage

WORKDIR /gomod
COPY go.mod go.sum ./
RUN go mod download

RUN mkdir -p /output

WORKDIR /kperf-build
RUN --mount=source=./,target=/kperf-build,rw make build && PREFIX=/output make install

# TODO: We should consider to implement our own curl to upload data
FROM ubuntu:22.04 AS release-stage

RUN apt update -y && apt install curl -y

WORKDIR /

COPY --from=build-stage /output/bin/kperf /kperf
COPY --from=build-stage /output/bin/runkperf /runkperf
COPY scripts/run_runner.sh /run_runner.sh
