package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/proxera/agent/pkg/nginx"
	"github.com/proxera/agent/pkg/types"
)

type Server struct {
	config  *types.AgentConfig
	manager *nginx.Manager
}

func NewServer(config *types.AgentConfig) *Server {
	manager := nginx.NewManager(
		config.NginxBinary,
		config.NginxConfigPath,
		config.NginxEnabledPath,
	)

	return &Server{
		config:  config,
		manager: manager,
	}
}

// requireAuth is a middleware that validates the Bearer token against the
// agent's configured API key. If no API key is configured, all requests
// are rejected with 403 to prevent accidental unauthenticated exposure.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.APIKey == "" {
			http.Error(w, `{"error":"no api_key configured, serve mode requires an api_key in the agent config"}`, http.StatusForbidden)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, `{"error":"authorization header required"}`, http.StatusUnauthorized)
			return
		}

		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, `{"error":"invalid authorization format, expected Bearer token"}`, http.StatusUnauthorized)
			return
		}

		token := authHeader[len(prefix):]
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.APIKey)) != 1 {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.requireAuth(s.handleHealth))
	mux.HandleFunc("/api/hosts", s.requireAuth(s.handleHosts))
	mux.HandleFunc("/api/reload", s.requireAuth(s.handleReload))

	addr := fmt.Sprintf(":%d", s.config.AgentPort)
	log.Printf("Agent API server starting on %s\n", addr)

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"agent_id": s.config.AgentID,
	})
}

func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.config.Hosts)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.manager.Test(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.manager.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "reloaded",
	})
}
