# Build stage: compile a static binary so the runtime image needs no toolchain.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/webhook-relay .

# Runtime stage: distroless-style scratch image with just the binary and certs,
# since the relay makes outbound HTTPS calls and needs a CA bundle.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/webhook-relay /usr/local/bin/webhook-relay
EXPOSE 8080
ENTRYPOINT ["webhook-relay"]
