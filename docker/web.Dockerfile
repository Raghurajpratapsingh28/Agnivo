# syntax=docker/dockerfile:1

FROM node:20-alpine AS base

ENV PNPM_HOME="/pnpm"
ENV PATH="${PNPM_HOME}:${PATH}"

RUN corepack enable && corepack prepare pnpm@10.12.1 --activate

# ── install deps ──────────────────────────────────────────────────────────────
FROM base AS deps

WORKDIR /app

COPY package.json pnpm-workspace.yaml turbo.json .npmrc ./
# Copy lockfile if present (not required — new installs work without it)
COPY pnpm-lock.yaml* ./
COPY apps/web/package.json ./apps/web/
# Copy any shared packages that web may depend on
COPY packages/ ./packages/

# No --frozen-lockfile: lockfile may not include the web workspace yet
RUN pnpm install --no-frozen-lockfile

# ── build ─────────────────────────────────────────────────────────────────────
FROM base AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/web/node_modules* ./apps/web/node_modules/
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1

RUN pnpm turbo run build --filter=@agnivo/web

# ── runtime ───────────────────────────────────────────────────────────────────
FROM base AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

# public dir is optional; only copy if it exists (Next.js standalone)
COPY --from=builder /app/apps/web/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/apps/web/.next/static ./apps/web/.next/static

USER nextjs

EXPOSE 3000

CMD ["node", "apps/web/server.js"]
