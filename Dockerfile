FROM golang:1.25

ENV GOPATH=/

ARG TARGETARCH
ARG MIGRATE_VERSION=v4.19.1

COPY ./ ./

RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql-client \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-${TARGETARCH}.tar.gz" \
    | tar xz -C /usr/local/bin migrate

RUN go mod download
RUN go build -o app ./cmd/main.go

CMD ["./app"]
