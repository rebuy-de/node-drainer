FROM quay.io/rebuy/rebuy-go-sdk:v2.4.0 as builder
ENV TZ=Europe/Berlin

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/dist/node-drainer /usr/local/bin/
