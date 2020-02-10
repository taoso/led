# ssltun

## 简介

优点很突出，采用标准协议，极其稳定！不需要客户端。

缺点则是性能略差，不支持移动端。而且还需要一个域名！

## 使用

首先安装 ssltun 命令
```
go get -u -v github.com/lvht/ssltun/cmd/ssltun
```

然后注册一个域名。**因是使用了标准 https 所以需要一个域名**。将域名解析到你的服务器IP。

我们这里假设使用 ssltun.io 作为域名。域名解析生效后启动 ssltun
```
# 默认使用 http/1.1 + tls 通信。
# 如果你的服务器网络很稳，可以使用`-h2`选项开启 http/2 通信。
ssltun -name ssltun.io -key foo
```

这里的 `-key` 参数用来指定用户名，别让人猜到。

ssltun 启动后会自动联系 letsencrypt 签发证书。

启动后访问 https://ssltun.io 你会看到**从中国发出的第一封邮件**的内容。

最后就是设置你的浏览器插件或者系统网络配置。
协议选`https`，域名填`ssltun.io`，端口填`443`，用户名填`foo`，密码随便写一个。

浏览器插件建议使用[SwitchyOmega](https://github.com/FelisCatus/SwitchyOmega)。
