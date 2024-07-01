FROM registry.access.redhat.com/ubi9/go-toolset:1.21.10-1.1719562237 as builder

WORKDIR /opt/app-root/src
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64  go build -a -o manager cmd/main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10-896.1717584414

COPY LICENSE /licenses
COPY --from=builder /opt/app-root/src/manager /
USER 65532:65532

ENTRYPOINT ["/manager"]
