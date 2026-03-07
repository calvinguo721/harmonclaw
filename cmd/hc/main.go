// Package main provides hc CLI for HarmonClaw admin.
package main

import (
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
		fmt.Println("hc: HarmonClaw CLI")
		fmt.Println("Usage: hc <command> [args]")
		fmt.Println("  hc health")
		fmt.Println("  hc skills")
		fmt.Println("  hc version")
		fmt.Println("  hc sovereign status")
		fmt.Println("  hc audit query")
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
			fmt.Println("hc audit query")
		}
	default:
		fmt.Printf("unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func req(path string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", strings.TrimSuffix(*baseURL, "/")+path, nil)
	if *token != "" {
		req.Header.Set("Authorization", "Bearer "+*token)
	}
	return http.DefaultClient.Do(req)
}

func doHealth() {
	resp, err := req("/v1/health")
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
	resp, err := req("/v1/architect/skills")
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
	resp, err := req("/v1/version")
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
	resp, err := req("/v1/governor/sovereignty")
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
	resp, err := req("/v1/audit/query")
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
