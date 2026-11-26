# AGENTS.md

写给在本仓库工作的 AI 编码助手。

## 项目

`led` 读作 **le-daemon**，是「乐乎」服务的后端。乐乎取《论语》「有朋自远方来，
不亦乐乎」之意，是一个让人发布内容、彼此往来的平台。

技术上它是多租户静态站点服务器：`-root` 下每个一级目录就是一个站点，目录名即
主机名。它只说纯 HTTP，不终结 TLS，跑在反向代理之后，由代理负责证书与主机名
白名单。

## 代码结构

| 文件 | 职责 |
|---|---|
| `led.go` | `LED`：Host 规范化、按目录惰性发现站点、`ServeMux` 路由、CORS |
| `file.go` | `FileHandler`、`leDir`（扩展名过滤）、`rewrite.txt` 重定向 |
| `alipay.go` | 支付宝下单与回调转发 |
| `letter.go` | `/+/mail` 评论邮件 |
| `push.go` | `/+/push` Web Push |
| `cmd/led/main.go` | 参数、监听、启动 |

`cmd/mkd/` 是 shell + lua + pandoc 写的博客静态化工具，与运行时代码没有调用关系。

## 常用命令

```
go build ./...
go vet ./...
gofmt -l .
```

仓库目前没有测试。

## 改动约定

1. **站点由目录发现**：没有站点清单文件，也不要加回 `-sites`。
2. **不做主机名校验**：过滤由前置代理负责，`led` 收到什么 Host 就服务什么，目录
   缺失则回落 `root/default`。
3. **依赖保持精简**：HTTP 代理、聊天计费、域名系统、WebDAV 等模块已移除，
   `webdav` / `autocert` / `quic-go` / `tiktoken` / `sqlite` 都不该再出现；路由用
   标准库 `http.ServeMux`，不要引入框架。
4. **不加访问日志**：启动时只打印一行监听地址。
5. **`env` 与 `.md` 不得对外提供**：这是 `leDir.Open` 的安全边界，改动该函数时
   必须保留；现有判断是大小写敏感的。
6. **`panic` 只用于启动期**不可恢复的配置错误，请求处理中一律返回 4xx/5xx。

## 提交

- 只提交被明确要求的改动，不要顺手格式化或重排无关代码。
- 提交信息用中文，一句话概括主题，细节放正文。
- 未明确要求时不要 `git commit` / `git push` / `git reset` / `git clean`。
