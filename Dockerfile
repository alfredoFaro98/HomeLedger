FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/homeledger ./cmd/homeledger

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/homeledger /app/homeledger
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/homeledger"]
