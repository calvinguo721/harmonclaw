// Package main provides hc CLI for HarmonClaw admin.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var (
	baseURL = flag.String("base", "http://localhost:8080", "API base URL")
	token   = flag.String("token", "", "Bearer token")
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("hc: HarmonClaw CLI v1.0")
		fmt.Println("Usage: hc <command> [args]")
		fmt.Println("  hc health              - 健康检查")
		fmt.Println("  hc skills              - 技能列表")
		fmt.Println("  hc version             - 版本信息")
		fmt.Println("  hc sovereign status    - 主权模式")
		fmt.Println("  hc audit query         - 审计查询")
		fmt.Println("  hc ledger [limit]      - 最新审计 (默认20)")
		fmt.Println("  hc chat <message>      - 发送对话")
		os.Exit(1)
	}
	cmd := args[0]
	switch cmd {
	case "health":
		doHealth()
	case "skills":
		doSkills()
	case "version":
		doVersion()
	case "sovereign":
		if len(args) > 1 && args[1] == "status" {
			doSovereign()
		} else {
			fmt.Println("hc sovereign status")
		}
	case "audit":
		if len(args) > 1 && args[1] == "query" {
			doAudit()
		} else {
			doAudit()
		}
	case "ledger":
		limit := 20
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &limit)
		}
		doLedger(limit)
	case "chat":
		if len(args) < 2 {
			fmt.Println("hc chat <message>")
			os.Exit(1)
		}
		doChat(strings.Join(args[1:], " "))
	default:
		fmt.Printf("unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func req(method, path string, body []byte) (*http.Response, error) {
	var r *http.Request
	var err error
	if body != nil {
		r, err = http.NewRequest(method, strings.TrimSuffix(*baseURL, "/")+path, bytes.NewReader(body))
	} else {
		r, err = http.NewRequest(method, strings.TrimSuffix(*baseURL, "/")+path, nil)
	}
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if *token != "" {
		r.Header.Set("Authorization", "Bearer "+*token)
	}
	return http.DefaultClient.Do(r)
}

func doHealth() {
	resp, err := req("GET", "/v1/health", nil)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}

func doSkills() {
	resp, err := req("GET", "/v1/architect/skills", nil)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}

func doVersion() {
	resp, err := req("GET", "/v1/version", nil)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}

func doSovereign() {
	resp, err := req("GET", "/v1/governor/sovereignty", nil)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}

func doAudit() {
	resp, err := req("GET", "/v1/audit/query", nil)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}

func doLedger(limit int) {
	path := fmt.Sprintf("/v1/ledger/latest?limit=%d", limit)
	resp, err := req("GET", path, nil)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d []map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}

func doChat(msg string) {
	body, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": msg}},
	})
	resp, err := req("POST", "/v1/chat/completions", body)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	if c, ok := d["choices"].([]any); ok && len(c) > 0 {
		if m, ok := c[0].(map[string]any); ok {
			if msg, ok := m["message"].(map[string]any); ok {
				fmt.Println(msg["content"])
				return
			}
		}
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))
}
