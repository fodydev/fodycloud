# FodyCloud control plane image.
FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY docs ./docs
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /out/fodycloud ./cmd/fodycloud

# The exact source this image is built from (the build context: everything
# OpenTofu ships, see terraform/files.tf), served at <domain>/source as
# required by the AGPL-3.0 (section 13).
FROM golang:1.27.4-alpine AS source
ARG VERSION=dev
COPY . /source/fodycloud-${VERSION}
RUN tar -czf /fodycloud-source.tar.gz -C /source "fodycloud-${VERSION}"

# Runtime: docker CLI + buildx plugin (to build and run student apps through the
# host daemon), git (clone public repositories) and the Railpack CLI (planning).
FROM docker:29.9.3-cli
ARG RAILPACK_VERSION=0.43.0
ARG RAILPACK_SHA256=248a8f60f36854aabed6124113631078a802de3461660855fc4de9c26065c1fb
RUN apk add --no-cache git ca-certificates tzdata \
 && wget -qO /tmp/railpack.tgz "https://github.com/railwayapp/railpack/releases/download/v${RAILPACK_VERSION}/railpack-v${RAILPACK_VERSION}-x86_64-unknown-linux-musl.tar.gz" \
 && echo "${RAILPACK_SHA256}  /tmp/railpack.tgz" | sha256sum -c - \
 && tar -xzf /tmp/railpack.tgz -C /usr/local/bin railpack \
 && rm /tmp/railpack.tgz \
 && railpack --version
COPY --from=build /out/fodycloud /usr/local/bin/fodycloud
COPY --from=source /fodycloud-source.tar.gz /usr/share/fodycloud/source.tar.gz
ENV FC_LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["fodycloud"]
CMD ["serve"]
