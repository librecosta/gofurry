ARG NODE_IMAGE=node:24.22.1-bookworm-slim
FROM ${NODE_IMAGE}
ENV NODE_ENV=production HOST=0.0.0.0 PORT=3000
WORKDIR /app
COPY --chown=node:node apps/web/.output .output
USER node
EXPOSE 3000
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=3 CMD ["node", "-e", "fetch('http://127.0.0.1:3000/readyz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]
CMD ["node", ".output/server/index.mjs"]
