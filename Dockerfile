FROM golang:1.22-alpine3.19 AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY . .

# The image should be built with
# --build-arg SG_VERSION=`git describe --tags --always`
ARG SG_VERSION
RUN if [ ! -z "$SG_VERSION" ]; then sed -i "s/UNKNOWN_RELEASE/${SG_VERSION}/>

RUN CGO_ENABLED=0 GOOS=linux go build \
        -ldflags "-s -w" \
        -a -o smtp-gotify





FROM alpine:3.19


RUN apk add --no-cache ca-certificates

COPY --from=builder /app/smtp-gotify /smtp-gotify

USER daemon

ENV SG_SMTP_LISTEN "0.0.0.0:2525"
EXPOSE 2525

ENTRYPOINT ["/smtp-gotify"]
