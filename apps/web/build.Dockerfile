ARG NODE_IMAGE=node:24.22.1-bookworm-slim
FROM ${NODE_IMAGE} AS build
WORKDIR /workspace
RUN npm install --global pnpm@12.8.1
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/web/package.json apps/web/package.json
RUN --mount=type=cache,target=/pnpm/store pnpm install --frozen-lockfile --store-dir /pnpm/store
COPY apps/web/ apps/web/
RUN pnpm --filter @gofurry/web build

FROM scratch AS artifacts
COPY --from=build /workspace/apps/web/.output/ /
