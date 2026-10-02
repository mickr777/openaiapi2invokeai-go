FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w \
      -X github.com/Pfannkuchensack/openaiapi2invokeai-go/internal/version.Version=${VERSION} \
      -X github.com/Pfannkuchensack/openaiapi2invokeai-go/internal/version.Commit=${COMMIT} \
      -X github.com/Pfannkuchensack/openaiapi2invokeai-go/internal/version.Date=${BUILD_DATE}" \
    -o /out/invoke-openai-proxy ./cmd/proxy

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata su-exec
COPY --from=build /out/invoke-openai-proxy /usr/local/bin/invoke-openai-proxy
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/invoke-openai-proxy /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /config

ENV PROXY_LISTEN_IP=0.0.0.0 \
    PROXY_PORT=8080 \
    PROXY_DATA_DIR=/config \
    PUID=99 \
    PGID=100

VOLUME ["/config"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/invoke-openai-proxy"]
