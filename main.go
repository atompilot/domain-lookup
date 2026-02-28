package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"
)

func main() {
	var (
		file        = flag.String("f", "", "从文件读取域名列表（每行一个）")
		concurrency = flag.Int("c", 5, "并发查询数")
		timeoutSec  = flag.Int("t", 10, "查询超时秒数")
		jsonMode    = flag.Bool("j", false, "JSON 格式输出")
		verbose     = flag.Bool("v", false, "详细模式（显示注册商、到期日、来源）")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: domain-lookup [选项] [域名...]\n\n")
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  domain-lookup example.com example.net\n")
		fmt.Fprintf(os.Stderr, "  domain-lookup -f domains.txt -j\n")
		fmt.Fprintf(os.Stderr, "  echo 'example.com' | domain-lookup\n")
	}
	flag.Parse()

	domains, err := readDomains(*file, flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	if len(domains) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	results := queryAll(domains, *concurrency, *timeoutSec)

	if *jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 输出错误:", err)
			os.Exit(1)
		}
		return
	}

	hasError := false
	for _, r := range results {
		printResult(r, *verbose)
		if r.Status == StatusUnknown {
			hasError = true
		}
	}
	if hasError {
		os.Exit(1)
	}
}

// readDomains 按优先级读取域名列表：文件 > stdin > 命令行参数。
func readDomains(file string, args []string) ([]string, error) {
	if file != "" {
		return readLines(file)
	}
	if isStdinPiped() {
		return scanLines(os.Stdin)
	}
	return args, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败 %s: %w", path, err)
	}
	defer f.Close()
	return scanLines(f)
}

func scanLines(r *os.File) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func isStdinPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

// queryAll 并发查询所有域名，保持输入顺序返回结果。
func queryAll(domains []string, concurrency, timeoutSec int) []Result {
	checker := NewChecker(timeoutSec)
	results := make([]Result, len(domains))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, domain := range domains {
		wg.Add(1)
		go func(i int, domain string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = checker.Check(domain)
		}(i, domain)
	}

	wg.Wait()
	return results
}

func printResult(r Result, verbose bool) {
	switch r.Status {
	case StatusAvailable:
		fmt.Printf("%-40s ✓ 未注册\n", r.Domain)

	case StatusRegistered:
		if verbose {
			extra := ""
			if r.Registrar != "" {
				extra += "  注册商: " + r.Registrar
			}
			if r.Expiry != nil {
				extra += "  到期: " + r.Expiry.Format(time.DateOnly)
			}
			extra += "  [" + r.Source + "]"
			fmt.Printf("%-40s ✗ 已注册%s\n", r.Domain, extra)
		} else {
			fmt.Printf("%-40s ✗ 已注册\n", r.Domain)
		}

	default:
		fmt.Fprintf(os.Stderr, "%-40s ? 查询失败: %s\n", r.Domain, r.Err)
	}
}
