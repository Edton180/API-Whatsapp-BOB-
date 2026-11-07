# API WhatsApp BOB - Desenvolvido por Edton Braz.
# Compilação com cache: numa atualização só o que mudou é baixado e recompilado.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache git build-base libwebp-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,id=bob-go-mod,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=""
RUN --mount=type=cache,id=bob-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=bob-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath \
      -ldflags "-s -w -X github.com/edton180/api-whatsapp-bob/internal/version.Version=${VERSION}" \
      -o /build/bob ./cmd/bob

FROM alpine:3.21
# ffmpeg: áudio vira nota de voz (ogg/opus)
RUN apk add --no-cache tzdata ffmpeg libwebp ca-certificates wget
WORKDIR /app
COPY --from=build /build/bob ./bob
COPY --from=build /build/VERSION /build/LICENSE /build/NOTICE ./
# Commit compilado (o OmniFlow compara com o repositório para saber se há novidade)
ARG COMMIT=""
RUN printf '%s' "${COMMIT}" > /app/COMMIT && mkdir -p /app/dbdata /app/logs
ENV TZ=America/Sao_Paulo SERVER_PORT=4000 DATA_DIR=/app/dbdata
EXPOSE 4000
VOLUME ["/app/dbdata"]
HEALTHCHECK --interval=15s --timeout=5s --retries=10 CMD wget -qO- http://127.0.0.1:4000/server/ok >/dev/null || exit 1
ENTRYPOINT ["/app/bob"]
