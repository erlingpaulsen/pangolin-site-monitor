# ---- build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /src

# Pre-cache modules (copy mod files first). This copies go.mod and, if present, go.sum.
COPY go.* ./
RUN go mod download && go mod verify

# Copy the rest of the source
COPY . .

# Make sure sums reflect actual imports
RUN go mod tidy

# Build static binary (no CGO)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags='-s -w' -o /out/pangolin-site-monitor .

# ---- minimal runtime ----
FROM gcr.io/distroless/static:nonroot
USER nonroot:nonroot
COPY --from=builder /out/pangolin-site-monitor /pangolin-site-monitor
ENTRYPOINT ["/pangolin-site-monitor"]