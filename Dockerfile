FROM golang:1.26.0-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
ARG GOPROXY=https://proxy.golang.org,direct
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/xchat-server ./cmd/xchat-server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/spacechat-release ./cmd/spacechat-release

# Inherit the module cache so client compilation needs no runtime network.
FROM go-build AS artifacts
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl openssl unzip zip \
 && rm -rf /var/lib/apt/lists/*
ARG WINDOWS_TERMINAL_URL=https://github.com/microsoft/terminal/releases/download/v1.24.11911.0/Microsoft.WindowsTerminal_1.24.11911.0_x64.zip
RUN install -m 0755 /out/xchat-server /out/spacechat-release /usr/local/bin/ \
 && curl --fail --location --retry 3 --connect-timeout 15 --max-time 300 --speed-limit 1024 --speed-time 30 \
      -o /tmp/terminal.zip "$WINDOWS_TERMINAL_URL" \
 && echo "7691efeb71c8dd0b95536c84e366fa4cf809a42c534912f9cefa1056534383bd  /tmp/terminal.zip" | sha256sum -c - \
 && unzip -q /tmp/terminal.zip -d /tmp/terminal \
 && mv /tmp/terminal/terminal-1.24.11911.0 /opt/windows-terminal \
 && rm -rf /tmp/terminal.zip /tmp/terminal
COPY README.md ./README.md
COPY deploy/client ./deploy/client
COPY scripts/windows-terminal ./scripts/windows-terminal
COPY --chmod=0755 deploy/docker/build-artifacts.sh ./deploy/docker/build-artifacts.sh
ENV GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local
ENTRYPOINT ["/src/deploy/docker/build-artifacts.sh"]

FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates su-exec \
 && addgroup -S -g 10001 spacechat \
 && adduser -S -D -H -u 10001 -G spacechat spacechat \
 && mkdir -p /data /run/spacechat /app \
 && chown spacechat:spacechat /data /run/spacechat \
 && chmod 0750 /data /run/spacechat
COPY --from=go-build /out/xchat-server /usr/local/bin/xchat-server
COPY internal/kaomoji/defaults.json /app/kaomoji.json
COPY deploy/docker/server-entrypoint.sh deploy/docker/healthcheck.sh /usr/local/bin/
RUN chmod 0755 /usr/local/bin/server-entrypoint.sh /usr/local/bin/healthcheck.sh
EXPOSE 18081
ENTRYPOINT ["/usr/local/bin/server-entrypoint.sh"]

FROM python:3.13-alpine3.22 AS cleanup
RUN addgroup -S -g 10001 spacechat \
 && adduser -S -D -H -u 10001 -G spacechat spacechat
WORKDIR /app
COPY deploy/clear_history.py ./clear_history.py
ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1
USER 10001:10001
ENTRYPOINT ["python3", "/app/clear_history.py"]
CMD ["--schedule-daily", "--socket", "/run/spacechat/admin.sock"]
