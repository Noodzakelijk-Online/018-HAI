# Angular 16 is validated on Node 20 LTS. Pin the toolchain so a future
# floating Alpine tag cannot silently move local Windows builds to an
# unsupported Node release.
FROM node:20.19.4-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./

RUN npm ci --prefer-offline --no-audit


COPY . .
RUN npm run build

# Stage 2: Serve the app with Nginx
FROM nginx:stable-alpine-slim
COPY --from=build /app/dist/app /usr/share/nginx/html

COPY ./nginx-custom.conf /etc/nginx/conf.d/default.conf
