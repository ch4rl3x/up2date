FROM golang:1.23 AS build

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY common ./common
COPY orchestrator ./orchestrator
COPY collector ./collector
COPY resolver ./resolver
COPY publisher ./publisher

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/up2date ./cmd/up2date

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/up2date /up2date

ENTRYPOINT ["/up2date"]
