FROM golang:1.21-alpine as builder

RUN apk add --no-cache git openssl

ENV CGO_ENABLED=0
RUN go install golang.org/x/lint/golint@latest

COPY . /build
RUN cd /build && ./buildutil

FROM alpine:latest
RUN apk add --no-cache ca-certificates && adduser -D node-drainer
COPY --from=builder /build/dist/node-drainer /usr/local/bin/
USER node-drainer
