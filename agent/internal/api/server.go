package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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

func (s *Server) Start() error {
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/api/hosts", s.handleHosts)
	http.HandleFunc("/api/reload", s.handleReload)

	addr := fmt.Sprintf(":%d", s.config.AgentPort)
	log.Printf("🚀 Agent API server starting on %s\n", addr)

	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
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
