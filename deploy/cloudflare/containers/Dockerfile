FROM golang:1.23.6-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY deploy/cloudflare/containers/container-src/healthz ./deploy/cloudflare/containers/container-src/healthz

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agent-svc-plus ./cmd/agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agent-healthz ./deploy/cloudflare/containers/container-src/healthz

FROM debian:bookworm-slim

ARG XRAY_VERSION=v25.10.15

ENV DEBIAN_FRONTEND=noninteractive
ENV XRAY_LOCATION_ASSET=/usr/local/share/xray

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip bash tini \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL -o /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-64.zip" \
    && unzip /tmp/xray.zip -d /tmp/xray \
    && install -m 0755 /tmp/xray/xray /usr/local/bin/xray \
    && mkdir -p /usr/local/share/xray \
    && install -m 0644 /tmp/xray/geoip.dat /usr/local/share/xray/geoip.dat \
    && install -m 0644 /tmp/xray/geosite.dat /usr/local/share/xray/geosite.dat \
    && rm -rf /tmp/xray /tmp/xray.zip

RUN mkdir -p /etc/agent /usr/local/etc/xray/templates /usr/local/etc/xray /var/run/agent-svc-plus

COPY --from=builder /out/agent-svc-plus /usr/local/bin/agent-svc-plus
COPY --from=builder /out/agent-healthz /usr/local/bin/agent-healthz

COPY config/xray.xhttp.template.json /usr/local/etc/xray/templates/xray.xhttp.template.json
COPY config/xray.tcp.template.json /usr/local/etc/xray/templates/xray.tcp.template.json
COPY deploy/cloudflare/containers/runtime/account-agent.container.yaml /etc/agent/account-agent.container.yaml
COPY deploy/cloudflare/containers/runtime/xray.bootstrap.json /etc/agent/xray.bootstrap.json
COPY deploy/cloudflare/containers/runtime/entrypoint.sh /usr/local/bin/container-entrypoint.sh
COPY deploy/cloudflare/containers/runtime/restart-xray.sh /usr/local/bin/restart-xray.sh
COPY deploy/cloudflare/containers/runtime/systemctl-shim.sh /usr/local/bin/systemctl

RUN chmod +x /usr/local/bin/container-entrypoint.sh /usr/local/bin/restart-xray.sh /usr/local/bin/systemctl

EXPOSE 8080 1443

ENV AGENT_CONFIG_TEMPLATE=/etc/agent/account-agent.container.yaml
ENV AGENT_CONFIG_PATH=/etc/agent/account-agent.yaml
ENV XRAY_CONFIG_PATH=/usr/local/etc/xray/config.json
ENV XRAY_BOOTSTRAP_PATH=/etc/agent/xray.bootstrap.json
ENV XRAY_PID_FILE=/var/run/agent-svc-plus/xray.pid
ENV XRAY_TCP_PID_FILE=/var/run/agent-svc-plus/xray-tcp.pid
ENV AGENT_PID_FILE=/var/run/agent-svc-plus/agent.pid
ENV HEALTH_PORT=8080

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/container-entrypoint.sh"]
