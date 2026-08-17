# The public website.
#
# Built from the repository root so the workspace and packages/types resolve.
#   docker build -f apps/web/Dockerfile .

FROM node:24-alpine AS deps
WORKDIR /repo
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/
COPY apps/web/package.json apps/web/
COPY packages/types/package.json packages/types/
RUN npm ci

FROM node:24-alpine AS build
WORKDIR /repo
COPY --from=deps /repo/node_modules ./node_modules
COPY . .
# BACKEND_URL and SERVICE_KEY are NOT build arguments. They are read at runtime
# inside route handlers; passing them here would bake a secret into an image
# layer that anyone with the image can read (CLAUDE.md §3).
RUN npm run build --workspace @samari/web

FROM node:24-alpine AS runtime
WORKDIR /app
RUN addgroup -S samari && adduser -S -G samari samari

# Next's standalone output carries only the modules actually imported, which is
# a fraction of node_modules (I18).
COPY --from=build --chown=samari:samari /repo/apps/web/.next/standalone ./
COPY --from=build --chown=samari:samari /repo/apps/web/.next/static ./apps/web/.next/static
COPY --from=build --chown=samari:samari /repo/apps/web/public ./apps/web/public

USER samari
ENV NODE_ENV=production HOSTNAME=0.0.0.0 PORT=3001
EXPOSE 3001

HEALTHCHECK --interval=15s --timeout=3s --start-period=15s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3001/ru >/dev/null || exit 1

CMD ["node", "apps/web/server.js"]
