# syntax=docker/dockerfile:1.7
#
# Next.js 15 with output: 'standalone'. Ships a minimal Node runtime that
# serves the leaderboard UI on port 3000.
#
#   docker buildx build --platform linux/arm64 -f web/Dockerfile -t iicpc/web:tag web/

FROM --platform=$BUILDPLATFORM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund

FROM --platform=$BUILDPLATFORM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ----------------------------------------------------------------------------
# Runtime: non-root, only the standalone output.
# ----------------------------------------------------------------------------
FROM node:22-alpine AS runtime
WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

RUN addgroup -S -g 65532 nextjs && \
    adduser -S -u 65532 -G nextjs nextjs

# Next standalone server + static assets. No public/ in this repo —
# add `COPY ... /app/public ./public` back if you introduce one.
COPY --from=build --chown=nextjs:nextjs /app/.next/standalone ./
COPY --from=build --chown=nextjs:nextjs /app/.next/static ./.next/static

USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
