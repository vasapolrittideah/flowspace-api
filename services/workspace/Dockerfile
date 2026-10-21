FROM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen ./gen
COPY internal ./internal
COPY services/workspace ./services/workspace
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workspace-api ./services/workspace/cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workspace-migrate ./services/workspace/cmd/migrate

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/workspace-api /workspace-api
COPY --from=build /out/workspace-migrate /workspace-migrate
USER 65532:65532
ENTRYPOINT ["/workspace-api"]
