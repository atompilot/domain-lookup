package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RDAPClient 使用 IANA RDAP Bootstrap 查询域名注册状态。
type RDAPClient struct {
	http      *http.Client
	mu        sync.RWMutex
	bootstrap map[string][]string // TLD → RDAP 服务器列表
	loaded    bool
}

func NewRDAPClient(timeoutSec int) *RDAPClient {
	return &RDAPClient{
		http:      &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		bootstrap: make(map[string][]string),
	}
}

// loadBootstrap 懒加载 IANA RDAP Bootstrap，仅执行一次。
func (c *RDAPClient) loadBootstrap() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return nil
	}

	resp, err := c.http.Get("https://data.iana.org/rdap/dns.json")
	if err != nil {
		return fmt.Errorf("获取 RDAP bootstrap 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Bootstrap JSON 格式：services 是 [[[tld,...], [server,...]], ...]
	var bf struct {
		Services [][][]string `json:"services"`
	}
	if err := json.Unmarshal(body, &bf); err != nil {
		return fmt.Errorf("解析 bootstrap JSON 失败: %w", err)
	}

	for _, svc := range bf.Services {
		if len(svc) < 2 {
			continue
		}
		tlds, servers := svc[0], svc[1]
		for _, tld := range tlds {
			c.bootstrap[strings.ToLower(tld)] = servers
		}
	}

	c.loaded = true
	return nil
}

// servers 根据域名查找对应的 RDAP 服务器列表，支持二级 TLD（如 co.uk）。
func (c *RDAPClient) servers(domain string) ([]string, error) {
	if err := c.loadBootstrap(); err != nil {
		return nil, err
	}

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("无效域名: %s", domain)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// 先尝试二级 TLD（如 co.uk）
	if len(parts) >= 3 {
		tld2 := strings.Join(parts[len(parts)-2:], ".")
		if srvs, ok := c.bootstrap[tld2]; ok {
			return srvs, nil
		}
	}

	// 再尝试顶级 TLD（如 com）
	tld := parts[len(parts)-1]
	if srvs, ok := c.bootstrap[tld]; ok {
		return srvs, nil
	}

	return nil, fmt.Errorf("未找到 TLD 的 RDAP 服务器: %s", tld)
}

// Query 查询域名注册状态，依次尝试各 RDAP 服务器。
func (c *RDAPClient) Query(domain string) (Result, error) {
	servers, err := c.servers(domain)
	if err != nil {
		return Result{}, err
	}

	for _, srv := range servers {
		url := strings.TrimSuffix(srv, "/") + "/domain/" + domain
		result, err := c.queryServer(url, domain)
		if err == nil {
			return result, nil
		}
	}

	return Result{}, fmt.Errorf("所有 RDAP 服务器均失败: %s", domain)
}

func (c *RDAPClient) queryServer(url, domain string) (Result, error) {
	resp, err := c.http.Get(url)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	// 404 表示域名未注册
	if resp.StatusCode == 404 {
		return Result{Domain: domain, Status: StatusAvailable}, nil
	}
	if resp.StatusCode != 200 {
		return Result{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	var d struct {
		LDHName  string   `json:"ldhName"`
		Status   []string `json:"status"`
		Events   []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Entities []struct {
			Roles      []string    `json:"roles"`
			VCardArray interface{} `json:"vcardArray"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return Result{}, fmt.Errorf("解析 RDAP 响应失败: %w", err)
	}

	result := Result{Domain: domain, Status: StatusRegistered}

	// 提取到期日
	for _, ev := range d.Events {
		if ev.Action == "expiration" {
			if t, err := time.Parse(time.RFC3339, ev.Date); err == nil {
				result.Expiry = &t
			}
		}
	}

	// 提取注册商名称（从 vcardArray 的 fn 字段）
	for _, entity := range d.Entities {
		for _, role := range entity.Roles {
			if role == "registrar" {
				result.Registrar = extractVCardFN(entity.VCardArray)
				break
			}
		}
	}

	return result, nil
}

// extractVCardFN 从 vcardArray 中提取 fn（全名）字段。
// vcardArray 格式：["vcard", [[prop, params, type, value], ...]]
func extractVCardFN(vc interface{}) string {
	arr, ok := vc.([]interface{})
	if !ok || len(arr) < 2 {
		return ""
	}
	props, ok := arr[1].([]interface{})
	if !ok {
		return ""
	}
	for _, p := range props {
		prop, ok := p.([]interface{})
		if !ok || len(prop) < 4 {
			continue
		}
		name, ok := prop[0].(string)
		if !ok || name != "fn" {
			continue
		}
		if val, ok := prop[3].(string); ok {
			return val
		}
	}
	return ""
}
