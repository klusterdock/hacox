FROM golang:1.21-bookworm

ARG VERSION=Unknown

WORKDIR /build

COPY . /build

RUN CGO_ENABLED=0 go build -mod vendor -buildmode=pie -ldflags "-X hacox/version.BuildVersion=${VERSION} -linkmode 'external' -extldflags '-static'" -o /opt/output/hacox cmd/main.go

FROM alpine:3.11
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WORKDIR /etc/hacox
COPY haproxy.cfg.tmpl /etc/hacox/
COPY --from=0 /opt/output/hacox /usr/local/bin/hacox

CMD [ "/usr/local/bin/hacox" ]
