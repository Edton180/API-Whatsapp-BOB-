# API WhatsApp BOB - baseada na Evolution Go (Evolution Foundation), Apache License 2.0.
# Compilação incremental: módulos e pacotes ficam em cache do BuildKit, então numa atualização só o
# que mudou é baixado e recompilado.
FROM golang:1.25.0-alpine AS build
RUN apk add --no-cache git build-base libjpeg-turbo-dev libwebp-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,id=bob-go-mod,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,id=bob-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=bob-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o server ./cmd/evolution-go

FROM alpine:3.20 AS final
# ffmpeg: áudio em nota de voz; poppler: miniatura de PDF
RUN apk add --no-cache tzdata ffmpeg libjpeg-turbo libwebp poppler-utils ca-certificates wget
WORKDIR /app
COPY --from=build /build/server ./server
COPY --from=build /build/VERSION /build/UPSTREAM /build/UPSTREAM_VERSION ./
COPY --from=build /build/LICENSE /build/NOTICE /build/TRADEMARKS.md ./
ENV TZ=America/Sao_Paulo
EXPOSE 4000
HEALTHCHECK --interval=15s --timeout=5s --retries=10 CMD wget -qO- http://127.0.0.1:4000/server/ok >/dev/null || exit 1
ENTRYPOINT ["/app/server"]
