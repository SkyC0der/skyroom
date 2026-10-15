FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY database ./database
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/skyroom-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/skyroom-api /skyroom-api
EXPOSE 8080
ENTRYPOINT ["/skyroom-api"]
