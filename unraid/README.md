# Unraid

The included template runs the Go proxy as a small bridge-network container and persists all user configuration under `/config`.

## Install

1. After an image has been published to GHCR, copy `invoke-openai-proxy.xml` to:
   `/boot/config/plugins/dockerMan/templates-user/my-invoke-openai-proxy.xml`
2. In Unraid, add the container from **User Templates**.
3. Set **InvokeAI URL** to the LAN address or container-resolvable address of InvokeAI, for example `http://192.168.1.20:9090`.
4. Open `http://UNRAID-IP:8081/admin`.

The default mapping is host port **8081** to container port **8080**.

## Multi-user InvokeAI

Set:

```text
INVOKE_AUTH_MODE=password
INVOKE_EMAIL=user@example.com
INVOKE_PASSWORD=your-password
INVOKE_VERSION=auto
```

The proxy logs in through InvokeAI's normal API login endpoint and re-authenticates once after a 401.

## Build locally instead of GHCR

From the repository root:

```bash
docker build -t invoke-openai-proxy:unraid .
```

Then change the Unraid template repository field to `invoke-openai-proxy:unraid`.
