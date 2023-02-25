# led

lehu daemon

led is a simple secure http proxy server with automic https.

If you need IP tunnel, you could use https://github.com/taoso/dtun

# quick start

Firstly, install the led
```
go get -u -v github.com/taoso/led/cmd/led
```

Secondly, register one domain name.

Suppose you have a domain named led.example. Your need to add an A record.

And then create a text file named sites.txt with the following content

```
led.example:
```

And then create a text file named users.txt with the following content

```
name:passwrd
```

The password need to be encrypted by bcrypt. You can use the htpasswd:

```
htpasswd -B -c ./users.txt foo
```

And then start the led,
```
sudo ./led -root /tmp -sites sites.txt -users users.txt
```

The option of `-root` is used for set static sites root dir.

All file in /tmp/led.example/ will be published to the Internet.

led uses https://letsencrypt.org so sign a https certificate automically.

Finally, set your system proxy or browser proxy extension using the **HTTPS** protocol.

We recommend to use the [SwitchyOmega](https://github.com/FelisCatus/SwitchyOmega)。

led will listen on 80/443 tcp port and 443 udp port for h3.
