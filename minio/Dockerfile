# Custom MinIO image built on a base without the x86-64-v2 CPU requirement.
#
# The official quay.io/minio/minio image moved to a UBI9 base whose glibc
# requires x86-64-v2, which crashes on older CPUs with:
#   "Fatal glibc error: CPU does not support x86-64-v2"
# (https://github.com/minio/minio/issues/18365).
#
# The MinIO project has since been archived and dl.min.io no longer serves
# prebuilt binaries (410 Gone), so we build the last released version from
# source instead of downloading it. Go itself has no CPU baseline
# requirement, and the resulting binary is statically linked (CGO disabled),
# so it runs on plain debian-slim without any glibc microarch requirement.
FROM golang:1.24-bookworm AS build

ARG MINIO_RELEASE=RELEASE.2025-10-15T17-29-55Z

RUN git clone --depth 1 --branch ${MINIO_RELEASE} \
    https://github.com/minio/minio.git /src
WORKDIR /src
RUN LDFLAGS="$(MINIO_RELEASE=RELEASE go run buildscripts/gen-ldflags.go)" \
    && CGO_ENABLED=0 GOAMD64=v1 go build -trimpath -ldflags "${LDFLAGS}" -o /out/minio .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/minio /usr/bin/minio

EXPOSE 9000 9090

ENTRYPOINT ["minio"]
