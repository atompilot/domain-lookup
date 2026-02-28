package main

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// whoisServers 是常用 TLD 对应的 WHOIS 服务器映射。
var whoisServers = map[string]string{
	"ac":   "whois.nic.ac",
	"ae":   "whois.aeda.net.ae",
	"ai":   "whois.nic.ai",
	"app":  "whois.nic.google",
	"au":   "whois.auda.org.au",
	"biz":  "whois.biz",
	"ca":   "whois.cira.ca",
	"cc":   "ccwhois.verisign-grs.com",
	"cn":   "whois.cnnic.cn",
	"co":   "whois.nic.co",
	"com":  "whois.verisign-grs.com",
	"de":   "whois.denic.de",
	"dev":  "whois.nic.google",
	"edu":  "whois.educause.edu",
	"fr":   "whois.nic.fr",
	"hk":   "whois.hkirc.hk",
	"info": "whois.afilias.net",
	"io":   "whois.nic.io",
	"jp":   "whois.jprs.jp",
	"kr":   "whois.kr",
	"me":   "whois.nic.me",
	"mobi": "whois.dotmobiregistry.net",
	"net":  "whois.verisign-grs.com",
	"org":  "whois.pir.org",
	"ru":   "whois.tcinet.ru",
	"sh":   "whois.nic.sh",
	"so":   "whois.nic.so",
	"tv":   "tvwhois.verisign-grs.com",
	"uk":   "whois.nic.uk",
	"us":   "whois.nic.us",
	"xyz":  "whois.nic.xyz",
}

// 域名未注册的特征字符串（小写匹配）
var notRegisteredMarkers = []string{
	"no match for",
	"not found",
	"no entries found",
	"no data found",
	"object does not exist",
	"no objects found",
	"domain not found",
	"status: free",
	"available for registration",
	"this domain name has not been registered",
}

// 域名已注册的特征字段（小写匹配）
var registeredMarkers = []string{
	"domain name:",
	"registrar:",
	"creation date:",
	"registered on:",
	"domain status:",
	"registrant:",
	"registry domain id:",
	"nserver:",
}

// WHOISClient 通过 TCP 43 端口查询 WHOIS。
type WHOISClient struct {
	timeout time.Duration
}

func NewWHOISClient(timeoutSec int) *WHOISClient {
	return &WHOISClient{timeout: time.Duration(timeoutSec) * time.Second}
}

func (c *WHOISClient) server(domain string) (string, error) {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("无效域名: %s", domain)
	}

	// 先尝试二级 TLD（如 co.uk）
	if len(parts) >= 3 {
		tld2 := strings.Join(parts[len(parts)-2:], ".")
		if srv, ok := whoisServers[tld2]; ok {
			return srv, nil
		}
	}

	tld := parts[len(parts)-1]
	if srv, ok := whoisServers[tld]; ok {
		return srv, nil
	}

	return "", fmt.Errorf("未找到 TLD 的 WHOIS 服务器: %s", tld)
}

// Query 通过 WHOIS 协议查询域名注册状态。
func (c *WHOISClient) Query(domain string) (Result, error) {
	srv, err := c.server(domain)
	if err != nil {
		return Result{}, err
	}

	conn, err := net.DialTimeout("tcp", srv+":43", c.timeout)
	if err != nil {
		return Result{}, fmt.Errorf("连接 WHOIS 服务器失败 %s: %w", srv, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(c.timeout)) //nolint:errcheck

	if _, err := fmt.Fprintf(conn, "%s\r\n", domain); err != nil {
		return Result{}, fmt.Errorf("发送 WHOIS 查询失败: %w", err)
	}

	body, err := io.ReadAll(conn)
	if err != nil {
		return Result{}, fmt.Errorf("读取 WHOIS 响应失败: %w", err)
	}

	result := parseWHOIS(domain, string(body))
	if result.Status == StatusUnknown {
		return result, fmt.Errorf("无法解析 WHOIS 响应: %s", domain)
	}
	return result, nil
}

func parseWHOIS(domain, body string) Result {
	lower := strings.ToLower(body)

	for _, marker := range notRegisteredMarkers {
		if strings.Contains(lower, marker) {
			return Result{Domain: domain, Status: StatusAvailable}
		}
	}

	for _, marker := range registeredMarkers {
		if strings.Contains(lower, marker) {
			result := Result{Domain: domain, Status: StatusRegistered}
			result.Registrar = whoisField(body, "Registrar")

			// 尝试多种到期日字段名
			for _, field := range []string{"Registry Expiry Date", "Expiry Date", "Expiration Date", "paid-till"} {
				if t := parseWhoisDate(whoisField(body, field)); !t.IsZero() {
					result.Expiry = &t
					break
				}
			}
			return result
		}
	}

	return Result{Domain: domain, Status: StatusUnknown}
}

// whoisField 提取 WHOIS 响应中指定字段的值（忽略大小写）。
func whoisField(body, field string) string {
	lowerField := strings.ToLower(field) + ":"
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), lowerField) {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

var whoisDateFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02",
	"02-Jan-2006",
	"2006.01.02",
	"02/01/2006",
}

func parseWhoisDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, f := range whoisDateFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
