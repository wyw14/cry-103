FROM golang:1.26.2

ENV GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local CGO_ENABLED=1
WORKDIR /workspace
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
RUN go build -mod=vendor -o /usr/local/bin/cableguard ./cmd/cableguard
CMD ["/usr/local/bin/cableguard"]
