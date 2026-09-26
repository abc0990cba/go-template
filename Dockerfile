# Build stage: static binaries, no cgo; build metadata is stamped here.
FROM golang:1.26-alpine AS build
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" -o /out/api ./cmd/api
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" -o /out/worker ./cmd/worker

# Runtime stage: minimal image, unprivileged user, healthcheck.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
COPY migrations /app/migrations
ENV APP_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/app/api", "-healthcheck"]
CMD ["/app/api"]
