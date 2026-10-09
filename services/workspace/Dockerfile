FROM golang:1.27.2-alpine@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673 AS build

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
