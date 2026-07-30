FROM golang:1.25-alpine AS builder

ARG GOSEC_VERSION=v2.28.0
RUN CGO_ENABLED=0 go install -ldflags "-X main.Version=${GOSEC_VERSION#v} -X main.GitTag=${GOSEC_VERSION}" github.com/securego/gosec/v2/cmd/gosec@${GOSEC_VERSION}

FROM golang:1.25-alpine
COPY --from=builder /go/bin/gosec /usr/local/bin/gosec
COPY app.py /app/app.py
RUN apk add --no-cache python3 && addgroup -g 10004 -S hai && adduser -S -D -H -u 10004 -G hai hai
USER 10004:10004
EXPOSE 8080
ENTRYPOINT ["python3", "/app/app.py"]
