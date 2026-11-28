# Multi-stage build for iERP Portfolio Cloud Run Job (Zero-CGO static binary)
FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/ierp ./cmd/ierp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/ierp /usr/local/bin/ierp
WORKDIR /tmp
ENV PORTFOLIO_DATA_DIR=/tmp/portfolio
ENTRYPOINT ["/usr/local/bin/ierp"]
CMD ["portfolio", "run", "--monthly-weekday"]
