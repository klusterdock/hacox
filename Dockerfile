FROM golang:1.23-alpine

ARG VERSION=Unknown

RUN apk add git build-base

WORKDIR /build

COPY . /build

RUN if [ "${VERSION}" = "Unknown" ]; then \
        VERSION=$(git describe --dirty --always --tags | sed 's/-/./g'); \
    fi; \
    CGO_ENABLED=0 go build -mod vendor \
        -ldflags "-s -w -X hacox/version.BuildVersion=${VERSION}" \
        -o /hacox cmd/main.go

RUN /hacox --version

FROM scratch
COPY --from=0 /hacox /hacox
ENTRYPOINT [ "/hacox" ]
