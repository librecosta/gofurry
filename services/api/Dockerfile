ARG GO_IMAGE=golang:1.27.3-bookworm
FROM ${GO_IMAGE} AS build
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY services/api/go.mod services/api/go.sum ./
RUN go mod download
COPY services/api .
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian13:nonroot AS runtime
COPY --from=build /out/api /api
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/api", "healthcheck", "--config", "/etc/gofurry/config.yaml"]
ENTRYPOINT ["/api"]
CMD ["--config", "/etc/gofurry/config.yaml"]
