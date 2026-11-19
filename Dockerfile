# syntax=docker/dockerfile:1
# Multi-arch: docker buildx build --platform linux/amd64,linux/arm64 .
# Builder stages run on the build machine's platform and cross-compile.

FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/commute . \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/commute /app
# Empty /data owned by the nonroot user, so a named volume starts out writable.
COPY --from=build --chown=65532:65532 /out/data /data
ENV TZ=Europe/Zurich LISTEN=:8080 DATA_DIR=/data CONFIG_PATH=/config/config.yaml
EXPOSE 8080
USER nonroot
HEALTHCHECK --interval=60s --timeout=5s --retries=3 CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
