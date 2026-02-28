# domain-lookup

检查域名是否已被注册，优先使用 RDAP 协议，失败时自动回退至 WHOIS。

## 特性

- 优先使用 [RDAP](https://datatracker.ietf.org/doc/html/rfc7480) 协议（HTTPS + 结构化 JSON）
- RDAP 不可用时自动回退到 WHOIS（TCP 43）
- 支持批量并发查询，默认 5 并发
- 三种输入方式：命令行参数、文件、stdin
- 双格式输出：人类可读文本 / JSON
- 零外部依赖，纯 Go 标准库

## 安装

### Homebrew（macOS / Linux，推荐）

```bash
brew tap atompilot/tap
brew install domain-lookup
```

### Go

```bash
go install github.com/atompilot/domain-lookup@latest
```

### 从源码编译

```bash
git clone https://github.com/atompilot/domain-lookup.git
cd domain-lookup
go build -o domain-lookup .
```

### Windows（Scoop）

```powershell
scoop bucket add atompilot https://github.com/atompilot/scoop-bucket
scoop install domain-lookup
```

## 使用

```
用法: domain-lookup [选项] [域名...]

选项:
  -f string   从文件读取域名列表（每行一个）
  -c int      并发数 (默认 5)
  -t int      查询超时秒数 (默认 10)
  -j          JSON 格式输出
  -v          详细模式（显示注册商、到期日、来源）
```

### 单个域名

```bash
domain-lookup example.com
```

### 批量查询（命令行参数）

```bash
domain-lookup example.com example.net mysite.io
```

### 从文件读取

```bash
domain-lookup -f domains.txt
```

### 从 stdin 读取（管道）

```bash
cat domains.txt | domain-lookup
echo "example.com" | domain-lookup
```

### JSON 输出（适合脚本处理）

```bash
domain-lookup -j -v google.com github.com
```

```json
[
  {
    "domain": "google.com",
    "status": "registered",
    "registrar": "MarkMonitor Inc.",
    "expiry": "2028-09-14T04:00:00Z",
    "source": "rdap"
  },
  {
    "domain": "github.com",
    "status": "registered",
    "registrar": "MarkMonitor Inc.",
    "expiry": "2026-10-09T18:20:50Z",
    "source": "rdap"
  }
]
```

### 详细文本输出

```bash
domain-lookup -v example.com mynewdomain.io
```

```
example.com                              ✗ 已注册  注册商: IANA  到期: 2026-08-13  [rdap]
mynewdomain.io                           ✓ 未注册
```

### 与 jq 配合使用

```bash
# 筛选出未注册的域名
domain-lookup -j -f domains.txt | jq -r '.[] | select(.status=="available") | .domain'

# 查看所有已注册域名的到期日
domain-lookup -j -f domains.txt | jq -r '.[] | select(.status=="registered") | "\(.domain)\t\(.expiry)"'
```

## Exit Code

| Code | 含义 |
|------|------|
| 0 | 所有域名查询成功（无论是否已注册） |
| 1 | 至少有一个域名查询失败（网络或解析错误） |

## 工作原理

```
输入域名
    │
    ▼
[1] 查询 IANA RDAP Bootstrap 获取注册局端点
    │
    ├─ 成功 ──▶ GET {rdap_server}/domain/{domain}
    │               ├─ HTTP 404 → 未注册
    │               └─ HTTP 200 → 已注册（提取注册商、到期日）
    │
    └─ 失败 ──▶ [2] TCP 连接 {whois_server}:43
                    ├─ 检测"未注册"关键词 → 未注册
                    └─ 检测"已注册"字段 → 已注册
```

## RDAP vs WHOIS

| 特性 | RDAP | WHOIS |
|------|------|-------|
| 协议 | HTTPS | TCP 43 |
| 响应格式 | 结构化 JSON | 非结构化文本 |
| 国际化 | 完整支持 | 有限 |
| 标准化 | RFC 7480/7483 | RFC 3912 |

## 参考

- [RFC 7480 - RDAP over HTTP](https://datatracker.ietf.org/doc/html/rfc7480)
- [IANA RDAP Bootstrap](https://data.iana.org/rdap/dns.json)
- [RFC 3912 - WHOIS Protocol](https://datatracker.ietf.org/doc/html/rfc3912)

## License

MIT
