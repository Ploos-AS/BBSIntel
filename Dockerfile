FROM golang:1.24-alpine AS build
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/Ploos-AS/BBSIntel/internal/buildinfo.Version=${VERSION} -X github.com/Ploos-AS/BBSIntel/internal/buildinfo.Commit=${VCS_REF} -X github.com/Ploos-AS/BBSIntel/internal/buildinfo.Date=${BUILD_DATE}" -o /out/bbsintel ./cmd/bbsintel

FROM alpine:3.22
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="BBSIntel" \
      org.opencontainers.image.description="Self-hosted BBS discovery, verification, monitoring, and intelligence service" \
      org.opencontainers.image.source="https://github.com/Ploos-AS/BBSIntel" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
RUN addgroup -S bbsintel && adduser -S -G bbsintel bbsintel && mkdir /data && chown bbsintel:bbsintel /data
COPY --from=build /out/bbsintel /usr/local/bin/bbsintel
USER bbsintel
VOLUME ["/data"]
EXPOSE 8080
ENV BBSINTEL_DB=/data/bbsintel.db BBSINTEL_LISTEN=:8080
ENTRYPOINT ["/usr/local/bin/bbsintel"]
