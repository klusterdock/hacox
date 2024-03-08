FROM golang:1.21-alpine

ARG VERSION=Unknown

RUN apk add git build-base

WORKDIR /build

COPY . /build

RUN if [ "${VERSION}" = "Unknown" ]; then \
        VERSION=$(git describe --dirty --always --tags | sed 's/-/./g'); \
    fi; \
    CGO_ENABLED=0 go build -mod vendor -buildmode=pie \
        -ldflags "-s -w -X hacox/version.BuildVersion=${VERSION} -linkmode 'external' -extldflags '-static'" \
        -o /opt/output/hacox cmd/main.go

FROM haproxy:2.9-alpine
WORKDIR /etc/hacox
COPY haproxy.cfg.tmpl /etc/hacox/
COPY --from=0 /opt/output/hacox /usr/local/bin/hacox

CMD [ "/usr/local/bin/hacox" ]
