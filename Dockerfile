FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/mcp-obsidian ./cmd/mcp-obsidian

FROM alpine:3.20
RUN adduser -D -h /home/mcp mcp && mkdir -p /vault && chown -R mcp:mcp /vault
USER mcp
WORKDIR /home/mcp
ENV OBSIDIAN_VAULT_PATH=/vault
VOLUME ["/vault"]
COPY --from=build /out/mcp-obsidian /usr/local/bin/mcp-obsidian
ENTRYPOINT ["/usr/local/bin/mcp-obsidian"]
