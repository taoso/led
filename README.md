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

Suppose you have a domain named led.example. You need to add an A record.

And then create a text file named sites.txt with the following content

```
led.example:
```

The site content is served from the `-root` directory: all files in
`/tmp/led.example/` will be published to the Internet. A site without its
own directory falls back to `root/default`.

And then start the led,
```
sudo ./led -root /tmp -sites sites.txt
```

led does not terminate TLS itself. Run it behind a reverse proxy and point the
proxy at the `-http1` listen address, binding it to localhost or an internal
interface.

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

Send `SIGHUP` to reload `sites.txt` without restarting.
