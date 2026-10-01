# syntax=docker/dockerfile:1.7

FROM golang:1.27.1-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/jungle-api ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=build /out/jungle-api /usr/local/bin/jungle-api

USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/jungle-api"]
