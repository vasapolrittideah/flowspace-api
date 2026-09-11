FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen ./gen
COPY services/workspace ./services/workspace
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workspace-api ./services/workspace/cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workspace-migrate ./services/workspace/cmd/migrate

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/workspace-api /workspace-api
COPY --from=build /out/workspace-migrate /workspace-migrate
USER 65532:65532
ENTRYPOINT ["/workspace-api"]
