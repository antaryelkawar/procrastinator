# syntax=docker/dockerfile:1

# Multi-stage build for the procrastinator-ui React + Vite SPA (PWA).
#
# Stage 1 ("build") compiles the TypeScript/Vite bundle on node:22-alpine.
# Stage 2 ships ONLY the static output on nginx:alpine — no node/npm, no source.
#
# Both base images are multi-arch (linux/amd64 + linux/arm64) and the build has
# no native cross-compile, so `docker buildx build --platform linux/amd64,linux/arm64`
# works with no special build ARGs.

# ---------------------------------------------------------------------------
# Stage 1: build the Vite production bundle
# ---------------------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /app

# Copy dependency manifests first so the `npm ci` layer is cached until the
# manifest changes. The build is a pure SPA build and needs no env vars
# (all import.meta.env.VITE_* usages are undefined-safe), so no ARG is required.
# components.json is copied for completeness (referenced by shadcn tooling).
COPY package.json package-lock.json components.json ./

# Install the exact locked dependency tree. `npm ci` is deterministic and
# fails on a lockfile/manifest mismatch — good for CI.
RUN npm ci

# Copy the remaining source (src/, public/, index.html, vite.config.ts,
# tsconfig*.json, nginx.conf, ...). .dockerignore excludes node_modules/ and
# dist/ so this broad copy stays lean.
COPY . .

# `tsc -b && vite build` -> /app/dist
RUN npm run build

# ---------------------------------------------------------------------------
# Stage 2: serve the static bundle with nginx
# ---------------------------------------------------------------------------
# No node/npm here — this stage only contains the built assets + nginx.
FROM nginx:alpine

# Vite's default output dir (dist/) -> nginx web root.
COPY --from=build /app/dist /usr/share/nginx/html

# Install the custom site config. nginx:alpine's base /etc/nginx/nginx.conf
# already provides the http{} context and `include /etc/nginx/conf.d/*.conf;`,
# so a server{}-only conf.d include composes cleanly with the base config.
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

# No ENTRYPOINT/CMD override: nginx:alpine's default CMD runs nginx as the
# master process (foreground) under Tini, which is what we want.
