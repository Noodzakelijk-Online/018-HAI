FROM node:24-bookworm-slim

WORKDIR /app

RUN npm install --global --ignore-scripts --no-audit --no-fund promptfoo@0.121.19

COPY app.mjs ./
COPY test_app.mjs ./

RUN useradd --create-home --uid 10001 --shell /usr/sbin/nologin hai

ENV HOME=/tmp \
    XDG_CACHE_HOME=/tmp/cache \
    PROMPTFOO_DISABLE_TELEMETRY=true \
    PROMPTFOO_DISABLE_UPDATE_CHECK=true \
    NODE_ENV=production

EXPOSE 8080

USER 10001:10001

CMD ["node", "app.mjs"]
