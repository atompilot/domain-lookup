# domain-lookup 设计文档

**日期：** 2026-03-01
**状态：** 已确认

## 需求

- 批量自动化场景：检查域名是否已被注册
- 三种输入源：命令行参数、`-f file`、stdin（优先级依次降低）
- 双格式输出：默认文本，`-j` 切换 JSON
- Exit Code：0 查询成功，1 有网络/解析错误
- 协议：RDAP 优先，失败回退 WHOIS

## 文件结构

```
domain-lookup/
├── go.mod
├── main.go      # CLI 入口：参数解析、输入读取、并发调度、输出格式
├── checker.go   # Result 类型 + Checker 编排（RDAP → WHOIS 回退）
├── rdap.go      # RDAP 客户端：IANA Bootstrap 缓存 + 域名查询
└── whois.go     # WHOIS 客户端：TLD→服务器映射 + TCP 查询 + 文本解析
```

## CLI 接口

```
用法: domain-lookup [选项] [域名...]

选项:
  -f string   从文件读取域名列表（每行一个）
  -c int      并发数 (默认 5)
  -t int      每次查询超时秒数 (默认 10)
  -j          JSON 格式输出
  -v          详细模式（显示注册商、到期日）

输入优先级: -f 文件 > stdin > 命令行参数
```

## 数据流

```
输入域名列表
    │
    ▼
[main] 创建 semaphore(c)，为每个域名启动 goroutine
    │
    ▼  并发执行
[checker.Check(domain)]
    ├─ rdap.Query() → 解析 RDAP JSON → Result{source:"rdap"}
    └─ (失败) whois.Query() → TCP 43 → 解析文本 → Result{source:"whois"}
    │
    ▼
收集全部 Result，按输入顺序排序
    ├─ -j 模式 → JSON 数组输出到 stdout
    └─ 默认 → 对齐文本表格输出到 stdout（错误 → stderr）
```

## Result 结构

```go
type Result struct {
    Domain    string
    Status    string    // "registered" | "available" | "unknown"
    Registrar string
    Expiry    time.Time
    Source    string    // "rdap" | "whois"
    Err       string    // 仅 unknown 时有值
}
```

## RDAP 客户端

1. 懒加载 IANA Bootstrap (`https://data.iana.org/rdap/dns.json`)，进程内缓存
2. 按 TLD 查找注册局 RDAP 端点，支持二级 TLD（如 `co.uk`）
3. HTTP 404 → available，200 → registered，其他 → error
4. 提取 registrar（entities vcardArray FN）和到期日（events expiration）

## WHOIS 客户端

1. 内置 TLD→服务器静态映射（~30 个常用 TLD）
2. TCP 连接 `{server}:43`，发送 `{domain}\r\n`
3. 先检测"未注册"关键词，再检测"已注册"字段
