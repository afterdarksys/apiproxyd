FROM golang:1.25-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=0.3.0
ARG COMMIT=dev
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=1 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/apiproxy .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 apiproxy \
    && mkdir -p /app \
    && chown apiproxy:apiproxy /app

COPY --from=build /out/apiproxy /usr/local/bin/apiproxy

USER apiproxy
WORKDIR /app
EXPOSE 9002

ENTRYPOINT ["/usr/local/bin/apiproxy"]
CMD ["daemon", "start"]
