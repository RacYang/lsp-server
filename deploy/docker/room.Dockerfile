ARG GO_VERSION=1.26.2

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

ENV CGO_ENABLED=0
ENV GOFLAGS=-mod=readonly

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/room ./cmd/room

FROM gcr.io/distroless/base-debian12

COPY --from=builder /out/room /room

USER nonroot:nonroot
ENTRYPOINT ["/room"]
