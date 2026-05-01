FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/telemetry-streamer ./cmd/telemetry-streamer

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/telemetry-streamer /app/telemetry-streamer
COPY dcgm_metrics_20250718_134233.csv /app/dcgm_metrics_20250718_134233.csv

ENV CSV_PATH=/app/dcgm_metrics_20250718_134233.csv

EXPOSE 8080

ENTRYPOINT ["/app/telemetry-streamer"]
