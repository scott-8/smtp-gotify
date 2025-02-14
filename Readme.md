# NOTE: This repository is not maintained, please consider using a fork

# SMTP to Gotify

`smtp-gotify` is a small program which listens for SMTP and sends
all incoming Email messages to your Gotify server. It is  modified version of 
[KostyaEsmukov/smtp_to_telegram ](https://github.com/KostyaEsmukov/smtp_to_telegram).

Say you have a software which can send Email notifications via SMTP.
You may use `smtp-gotify` as an SMTP server so
the notification mail can be sent to a Gotify app.

## Getting started

Starting a docker container:

```
docker run \
    --name smtp-gotify \
    -e GOTIFY_URL=<SERVER_URL> \
    -e GOTIFY_TOKEN=<APP_TOKEN1>,<APP_TOKEN2> \
    -p 2525:2525 \
    piedelivery/smtp-gotify
```

The variable `GOTIFY_URL` should be in the form `http[s]://example.com[:port]/`.

A few other environmental variables that can be optionally specified:
`GOTIFY_PRIORITY`, `GOTIFY_TITLE_TEMPLATE`, and `GOTIFY_MESSAGE_TEMPLATE`.

Assuming that your Email-sending software is running in docker as well,
you may use `smtp-gotify:2525` as the target SMTP address.
No TLS or authentication is required.
