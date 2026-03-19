// Package gateway provides CORS and security headers.
package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type securityConfig struct {
	CORS struct {
		AllowedOrigins []string `json:"allowed_origins"`
		AllowedMethods string   `json:"allowed_methods"`
		AllowedHeaders string   `json:"allowed_headers"`
		MaxAge         int      `json:"max_age"`
	} `json:"cors"`
	CSP string `json:"csp"`
}

func loadSecurityConfig() securityConfig {
	cfg := securityConfig{
		CSP: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'",
	}
	cfg.CORS.AllowedOrigins = []string{"*"}
	cfg.CORS.AllowedMethods = "GET, POST, PUT, DELETE, OPTIONS"
	cfg.CORS.AllowedHeaders = "Content-Type, Authorization, X-Requested-With"
	cfg.CORS.MaxAge = 86400

	paths := []string{"configs/security.json"}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, "configs/security.json"))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		json.Unmarshal(data, &cfg)
		if cfg.CORS.AllowedMethods == "" {
			cfg.CORS.AllowedMethods = "GET, POST, PUT, DELETE, OPTIONS"
		}
		break
	}
	return cfg
}

// CORS wraps handler with CORS headers.
func CORS(next http.Handler) http.Handler {
	cfg := loadSecurityConfig()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowStar := false
		allowOrigin := ""
		for _, o := range cfg.CORS.AllowedOrigins {
			if o == "*" {
				allowStar = true
				break
			}
			if o == origin {
				allowOrigin = origin
				break
			}
		}
		if allowStar {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", cfg.CORS.AllowedMethods)
		w.Header().Set("Access-Control-Allow-Headers", cfg.CORS.AllowedHeaders)
		if cfg.CORS.MaxAge > 0 {
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.CORS.MaxAge))
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders adds CSP and other security headers.
func securityHeaders(next http.Handler) http.Handler {
	cfg := loadSecurityConfig()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.CSP != "" {
			w.Header().Set("Content-Security-Policy", cfg.CSP)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

// CORSForV1 applies CORS only to /v1/ paths.
func CORSForV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
