# ssltun

ssltun is a simple secure http proxy server with automic https.

# quick start

Firstly, install the ssltun
```
go get -u -v github.com/lvht/ssltun/cmd/ssltun
```

Secondly, register one domain name.

Suppose we have a domain ssltun.io. And add an A record to you server ip.
And then start the ssltun,
```
# http/1.1 + tls is used as default.
# You can use the `-h2` option to enable http/2
ssltun -name ssltun.io -key foo
```

The option of `-key` is used for set one username for authentication.

ssltun will use [letsencrypt]() so sign a https certificate automically。

Then you can browse the http://ssltun.io, you will read
> Across the Great Wall we can reach every corner in the world.

Finally, set your system proxy or browser proxy extension using the **HTTPS** protocol.

We recommend to use the [SwitchyOmega](https://github.com/FelisCatus/SwitchyOmega)。
