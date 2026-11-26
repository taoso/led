# led

le-daemon，乐乎服务的后端。

「乐乎」取自《论语》「有朋自远方来，不亦乐乎」——一个让人发布内容、彼此往来的
地方。`led` 即 **le-daemon**，是这套服务的常驻后端进程，一个多租户静态站点
服务器：它只说纯 HTTP，本身不终结 TLS，设计上跑在反向代理（nginx 等）后面，由
代理负责证书与主机名白名单。

# 快速开始

先安装：

```
go get -u -v github.com/taoso/led/cmd/led
```

准备一个域名，例如 `led.example`，给它加一条 A 记录，并让前面的反向代理把该
域名转发到 led 的 `-http1` 地址。

站点内容放在 `-root` 目录下，**一级子目录名就是主机名**：

```
/tmp/led.example/index.htm       -> https://led.example/
/tmp/default/index.htm           -> 未建目录的站点共用这个兜底页
```

启动：

```
sudo ./led -root /tmp -http1 127.0.0.1:8080
```

led 不读取站点清单，也不做主机名过滤：收到什么 Host 就按同名目录提供内容，目录
不存在则回落到 `root/default`。因此 `-http1` 应当只绑在 127.0.0.1 或内网地址上，
靠代理来拦截未知域名。

新增或删除站点目录**即时生效**，无需重启。

# 站点功能

每个站点对应 `-root` 下的一个目录，以主机名对外提供服务。

## rewrite.txt

站点目录下可放 `rewrite.txt`，每行一条规则，格式为 `旧路径 -> 新路径`，先匹配
先生效：

```
/old/ -> /new/
@/tmp/ -> /short-lived/     # 前缀 @ 表示返回 307 临时重定向
~/blog -> https://example.org # 前缀 ~ 表示把原始请求 URI 拼到目标后面
```

不带前缀时为 301 永久重定向。`/ads.txt` 不参与重写。

## 路径约定

- 以 `.md` 结尾的路径、以及名为 `env` 的文件一律不对外提供（`env` 里存着站点
  的 SMTP/推送密钥）。
- 请求 `/foo.html` 时若 `/foo.htm` 存在，则直接返回 `foo.htm`。
- 目录下没有 `index.htm`/`index.html` 时返回 404；若存在 `.autoindex` 文件，
  则允许列出目录内容。
- 请求任意 `/a/index.htm` 会 301 重定向到 `./`。

## 接口

| 路径 | 方法 | 说明 |
|---|---|---|
| `/+/mail` | POST | 评论表单，通过 SMTP 发给 `root/<host>/env` 里配置的作者地址 |
| `/+/push` | POST | Web Push 通知，密钥取自 `root/<host>/env` |
| `/+/alipay-order-create` | POST | 支付宝扫码下单，请求需用 ed25519 应用私钥签名 |
| `/+/alipay-order-notify` | 任意 | 支付宝异步通知，验签后转发给下单时给出的地址 |

支付宝接口的请求头：`zz-app` 为应用名，`zz-sign` 为对请求体的 ed25519 签名；
服务端用环境变量 `APP_PUBKEY_<应用名>` 验签。下单成功返回 `{"qr": ..., "ttl": 900}`。

## 其它行为

- 只有当 `Origin` 里的主机名**有真实目录**时，才回
  `Access-Control-Allow-Origin: *`；走 `default` 兜底的站点不放行 CORS。
- Host 会先规范化：去掉端口、转小写、去掉 FQDN 末尾的点，再做 IDNA 转换，因此
  `/tmp/例子.example/` 这种 Unicode 目录名可以正常访问。
- led 不写访问日志，启动时只打一行监听地址。

# 环境变量

| 变量 | 用途 |
|---|---|
| `SMTP_USER` / `SMTP_PASS` / `SMTP_HOST` | `/+/mail` 发信 |
| `ALIPAY_APP_ID` / `ALIPAY_PRIVATE_KEY` / `ALIPAY_PUBLIC_KEY` | 支付宝客户端 |
| `APP_PUBKEY_<app>` | 调用支付宝下单接口的应用公钥 |
