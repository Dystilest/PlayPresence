# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/Dystilest/PlayPresence/internal/version.Version=${VERSION} -X github.com/Dystilest/PlayPresence/internal/version.Commit=${COMMIT}" \
    -o /out/playpresence ./cmd/playpresence

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/playpresence /playpresence
# Discord's IPC socket must be mounted at /tmp/discord-ipc-0 (see docs/installation.md).
USER nonroot:nonroot
ENTRYPOINT ["/playpresence"]
CMD ["version"]
