// Package update serves the locally built Android package to devices on the LAN.
package update

import "preferans/locales"

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type manifest struct {
	APK string `json:"apk"`
}

type Server struct {
	mu         sync.RWMutex
	aiTransfer http.Handler
	httpServer *http.Server
	listener   net.Listener
}

// Start listens on all network interfaces and serves only version.json and the
// APK named by that manifest. It is intended for a trusted home/LAN network.
func Start(root string, port int) (*Server, error) {
	if port < 0 || port > 65535 {
		return nil, locales.Errorf("go.internal.update.server.text001", port)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	setHeaders := func(w http.ResponseWriter) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Android WebView may enforce Private Network Access when the game UI
		// (served from localhost) requests the update server on the LAN.
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	}
	mux.HandleFunc("/version.json", func(w http.ResponseWriter, r *http.Request) {
		setHeaders(w)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "version.json"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setHeaders(w)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" || strings.Contains(strings.TrimPrefix(r.URL.Path, "/"), "/") {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filepath.Join(root, "version.json"))
		if err != nil {
			http.Error(w, "version manifest is not available", http.StatusNotFound)
			return
		}
		var m manifest
		if json.Unmarshal(data, &m) != nil || m.APK == "" || filepath.Base(m.APK) != m.APK {
			http.Error(w, "invalid version manifest", http.StatusInternalServerError)
			return
		}
		if "/"+m.APK != r.URL.Path {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, m.APK))
	})
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, err
	}
	s := &Server{listener: listener}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ai-transfer/") {
			s.mu.RLock()
			transfer := s.aiTransfer
			s.mu.RUnlock()
			if transfer == nil {
				http.NotFound(w, r)
			} else {
				transfer.ServeHTTP(w, r)
			}
			return
		}
		if r.Method == http.MethodOptions {
			setHeaders(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
	s.httpServer = &http.Server{Handler: handler}
	go func() { _ = s.httpServer.Serve(listener) }()
	return s, nil
}

func (s *Server) SetAITransfer(handler http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiTransfer = handler
}

func (s *Server) Close() error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
