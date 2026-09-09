FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bbsintel ./cmd/bbsintel

FROM alpine:3.22
RUN addgroup -S bbsintel && adduser -S -G bbsintel bbsintel && mkdir /data && chown bbsintel:bbsintel /data
COPY --from=build /out/bbsintel /usr/local/bin/bbsintel
USER bbsintel
VOLUME ["/data"]
EXPOSE 8080
ENV BBSINTEL_DB=/data/bbsintel.db BBSINTEL_LISTEN=:8080
ENTRYPOINT ["/usr/local/bin/bbsintel"]
