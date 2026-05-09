# syntax=docker/dockerfile:1.4

FROM smallstep/step-cli:0.27.0 AS step

FROM golang:1.25-alpine AS builder
WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./

RUN --mount=type=secret,id=github_token,required=false \
    if [ -f /run/secrets/github_token ]; then \
      TOKEN=$(cat /run/secrets/github_token) && \
      git config --global url."https://${TOKEN}@github.com/".insteadOf "https://github.com/" && \
      GONOSUMDB="github.com/devspotai/*" go mod download && \
      rm -f /root/.gitconfig; \
    fi

COPY . .

RUN if [ -d vendor ]; then \
      go build -mod=vendor -o server ./cmd; \
    else \
      go build -mod=mod -o server ./cmd; \
    fi

FROM alpine:3.23
WORKDIR /app

COPY --from=builder /build/server /app/server
COPY --from=step /usr/local/bin/step /usr/local/bin/step

RUN apk add --no-cache wget

COPY start.sh /app/start.sh
RUN chmod +x /app/start.sh

CMD ["/app/start.sh"]
