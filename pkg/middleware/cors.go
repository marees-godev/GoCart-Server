package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-Request-ID", "X-Admin-Key"},
		ExposedHeaders:   []string{"Content-Length", "Access-Control-Allow-Origin", "Access-Control-Allow-Headers"},
		AllowCredentials: false,
		MaxAge:           86400,
	}
}

func CORS(config ...CORSConfig) func(http.Handler) http.Handler {
	cfg := DefaultCORSConfig()
	if len(config) > 0 {
		userCfg := config[0]
		if len(userCfg.AllowedOrigins) > 0 {
			cfg.AllowedOrigins = userCfg.AllowedOrigins
		}
		if len(userCfg.AllowedMethods) > 0 {
			cfg.AllowedMethods = userCfg.AllowedMethods
		}
		if len(userCfg.AllowedHeaders) > 0 {
			cfg.AllowedHeaders = userCfg.AllowedHeaders
		}
		if len(userCfg.ExposedHeaders) > 0 {
			cfg.ExposedHeaders = userCfg.ExposedHeaders
		}
		cfg.AllowCredentials = userCfg.AllowCredentials
		if userCfg.MaxAge > 0 {
			cfg.MaxAge = userCfg.MaxAge
		}
	}

	allowedMethodsStr := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeadersStr := strings.Join(cfg.AllowedHeaders, ", ")
	exposedHeadersStr := strings.Join(cfg.ExposedHeaders, ", ")
	maxAgeStr := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowedOrigin := ""
			for _, o := range cfg.AllowedOrigins {
				if o == "*" {
					if cfg.AllowCredentials {
						allowedOrigin = origin
					} else {
						allowedOrigin = "*"
					}
					break
				}
				if strings.EqualFold(o, origin) {
					allowedOrigin = origin
					break
				}
			}

			if allowedOrigin == "" {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			if allowedOrigin != "*" {
				w.Header().Add("Vary", "Origin")
			}

			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if exposedHeadersStr != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposedHeadersStr)
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethodsStr)

				reqHeaders := r.Header.Get("Access-Control-Request-Headers")
				if reqHeaders != "" {
					w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
				} else if allowedHeadersStr != "" {
					w.Header().Set("Access-Control-Allow-Headers", allowedHeadersStr)
				}

				if cfg.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", maxAgeStr)
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
