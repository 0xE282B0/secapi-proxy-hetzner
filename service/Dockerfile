FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/secapi-proxy-hetzner ./cmd/secapi-proxy-hetzner

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/secapi-proxy-hetzner /secapi-proxy-hetzner

EXPOSE 8080
ENTRYPOINT ["/secapi-proxy-hetzner"]
