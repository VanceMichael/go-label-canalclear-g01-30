# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/canalclear ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/canalclear /canalclear
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/canalclear"]
