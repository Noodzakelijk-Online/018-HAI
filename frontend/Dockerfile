# Angular 22 requires Node 22.22.3 or newer. Pin the toolchain so a future
# floating Alpine tag cannot silently move local Windows builds to an
# unsupported Node release.
FROM node:22.22.3-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./

RUN --mount=type=cache,target=/root/.npm \
    npm ci --prefer-offline --no-audit --no-fund


COPY . .
RUN npm run build

# Stage 2: Serve the app with Nginx
FROM nginx:stable-alpine-slim
COPY --from=build /app/dist/app /usr/share/nginx/html

COPY ./nginx-custom.conf /etc/nginx/conf.d/default.conf
