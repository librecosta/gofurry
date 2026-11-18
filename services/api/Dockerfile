FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
ARG TARGETARCH
WORKDIR /app
COPY --chmod=0555 bin/linux/${TARGETARCH}/api /api
COPY services/api/migrations/*.sql /app/migrations/
USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/api", "healthcheck", "--config", "/etc/gofurry/config.yaml"]
ENTRYPOINT ["/api"]
CMD ["--config", "/etc/gofurry/config.yaml"]
