FROM node:22-bookworm-slim AS web-build
WORKDIR /src
COPY package.json package-lock.json* ./
COPY apps/web/package.json ./apps/web/package.json
RUN npm install
COPY apps/web ./apps/web
RUN npm run build:web

FROM caddy:2.10-alpine
COPY --from=web-build /src/apps/web/out /srv
