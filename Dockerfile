# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/skullblack ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build /out/skullblack /app/skullblack
COPY templates ./templates
COPY static ./static

ENV PORT=12080
EXPOSE 12080

USER nonroot:nonroot
ENTRYPOINT ["/app/skullblack"]
