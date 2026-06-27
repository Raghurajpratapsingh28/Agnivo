# syntax=docker/dockerfile:1

FROM node:20-alpine

ENV PNPM_HOME="/pnpm"
ENV PATH="${PNPM_HOME}:${PATH}"

RUN corepack enable && corepack prepare pnpm@10.12.1 --activate

WORKDIR /app

EXPOSE 3000

CMD ["sh", "-c", "pnpm install && pnpm turbo run dev --filter=@agnivo/web"]
