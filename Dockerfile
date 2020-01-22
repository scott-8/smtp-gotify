FROM golang:1.13-alpine3.10 AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY . .

# The image should be built with
# --build-arg SG_VERSION=`git describe --tags --always`
ARG SG_VERSION
RUN if [ ! -z "$SG_VERSION" ]; then sed -i "s/UNKNOWN_RELEASE/${SG_VERSION}/g" smtp-gotify.go; fi

RUN CGO_ENABLED=0 GOOS=linux go build \
        -ldflags "-s -w" \
        -a -o smtp-gotify





FROM alpine:3.10

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/smtp-gotify /smtp-gotify

USER daemon

ENV SG_SMTP_LISTEN "0.0.0.0:2525"
EXPOSE 2525

ENTRYPOINT ["/smtp-gotify"]
