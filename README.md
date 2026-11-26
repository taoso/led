# led

lehu daemon

led is a multi-tenant static site server. It speaks plain http and is meant to
run behind a reverse proxy which terminates TLS.

# quick start

Firstly, install the led
```
go get -u -v github.com/taoso/led/cmd/led
```

Secondly, register one domain name.

Suppose you have a domain named led.example. You need to add an A record, and
the reverse proxy in front of led must forward it to led's `-http1` address.

The site content is served from the `-root` directory: every first level
directory is a site, named after its host. All files in `/tmp/led.example/`
will be published to `https://led.example/`. A site without its own directory
falls back to `root/default`. Adding or removing a directory takes effect
immediately, no restart needed.

And then start the led,
```
sudo ./led -root /tmp -http1 127.0.0.1:8080
```

led does not terminate TLS itself. Run it behind a reverse proxy and point the
proxy at the `-http1` listen address, binding it to localhost or an internal
interface. The proxy is expected to filter hosts; led serves whatever host it
receives and keeps no site list of its own.

# site features

Every site owns a directory under `-root` and is published at its host name.

- `rewrite.txt` — one rule per line, `old/path -> new/path`. Prefix `@` for a
  temporary (307) redirect, prefix `~` to append the original request URI.
  The first matching rule wins.
- `/+/mail` — `POST` a comment form, delivered by SMTP to the author address
  configured in `root/<host>/env`.
- `/+/push` — Web Push notification, keys taken from `root/<host>/env`.
- `/+/alipay-order-create`, `/+/alipay-order-notify` — alipay QR order API for
  site owners, requests signed with an ed25519 app key (`APP_PUBKEY_<app>`).

# environment

| variable | purpose |
|---|---|
| `SMTP_USER` / `SMTP_PASS` / `SMTP_HOST` | outgoing mail for `/+/mail` |
| `ALIPAY_APP_ID` / `ALIPAY_PRIVATE_KEY` / `ALIPAY_PUBLIC_KEY` | alipay client |
| `APP_PUBKEY_<app>` | ed25519 public key of an alipay API caller |
