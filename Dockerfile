FROM quay.io/rebuy/rebuy-go-sdk:v3.5.2 as builder

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/dist/node-drainer /usr/local/bin/
