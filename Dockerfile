# ---- build stage ----
FROM golang:1.27-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/pangolin-site-monitor .

# ---- minimal runtime ----
FROM gcr.io/distroless/static:nonroot
USER nonroot:nonroot
COPY --from=builder /out/pangolin-site-monitor /pangolin-site-monitor
ENTRYPOINT ["/pangolin-site-monitor"]
