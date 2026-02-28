package main

import (
	"strings"
	"time"
)

const (
	StatusRegistered = "registered"
	StatusAvailable  = "available"
	StatusUnknown    = "unknown"
)

// Result 表示单个域名的查询结果。
type Result struct {
	Domain    string     `json:"domain"`
	Status    string     `json:"status"`
	Registrar string     `json:"registrar,omitempty"`
	Expiry    *time.Time `json:"expiry,omitempty"`
	Source    string     `json:"source,omitempty"`
	Err       string     `json:"error,omitempty"`
}

// Checker 按 RDAP → WHOIS 顺序查询域名注册状态。
type Checker struct {
	rdap  *RDAPClient
	whois *WHOISClient
}

func NewChecker(timeoutSec int) *Checker {
	return &Checker{
		rdap:  NewRDAPClient(timeoutSec),
		whois: NewWHOISClient(timeoutSec),
	}
}

func (c *Checker) Check(domain string) Result {
	domain = strings.ToLower(strings.TrimSpace(domain))

	if result, err := c.rdap.Query(domain); err == nil {
		result.Source = "rdap"
		return result
	}

	result, err := c.whois.Query(domain)
	if err != nil {
		return Result{Domain: domain, Status: StatusUnknown, Err: err.Error()}
	}
	result.Source = "whois"
	return result
}
