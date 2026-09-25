FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY contracts/events ./contracts/events
COPY gen ./gen
COPY internal ./internal
COPY services/identity ./services/identity
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/identity-api ./services/identity/cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/identity-worker ./services/identity/cmd/worker && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/identity-migrate ./services/identity/cmd/migrate

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/identity-api /identity-api
COPY --from=build /out/identity-worker /identity-worker
COPY --from=build /out/identity-migrate /identity-migrate
USER 65532:65532
ENTRYPOINT ["/identity-api"]
