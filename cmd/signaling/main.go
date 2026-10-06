package main

// Persistent room directory, temporary rendezvous, and authenticated admin site.
import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"preferans/internal/adminweb"
	"preferans/internal/rooms"
	"strings"
	"sync"
	"time"
)

type envelope struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
	SDP  string `json:"sdp,omitempty"`
	Code string `json:"code,omitempty"`
	Seat int    `json:"seat,omitempty"`
}
type singleton struct {
	mu      sync.Mutex
	offer   envelope
	answer  envelope
	updated time.Time
}

func (s *singleton) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodPost {
		var e envelope
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&e) != nil || e.Kind == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.updated = time.Now()
		switch e.Kind {
		case "offer":
			s.offer = e
			s.answer = envelope{}
		case "answer":
			s.answer = e
		case "clear":
			s.offer = envelope{}
			s.answer = envelope{}
		default:
			http.Error(w, "bad kind", 400)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if time.Since(s.updated) > 10*time.Minute {
		s.offer = envelope{}
		s.answer = envelope{}
	}
	_ = json.NewEncoder(w).Encode(struct {
		Offer  envelope `json:"offer"`
		Answer envelope `json:"answer"`
	}{s.offer, s.answer})
}
func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	data := flag.String("data", "./signaling-data", "Persistent room directory")
	reset := flag.Bool("admin-password-stdin", false, "Reset admin password from stdin and exit")
	cert := flag.String("tls-cert", "", "Optional TLS certificate")
	key := flag.String("tls-key", "", "Optional TLS key")
	flag.Parse()
	r, err := rooms.New(*data)
	if err != nil {
		log.Fatal(err)
	}
	admin, err := adminweb.New(*data, r)
	if err != nil {
		log.Fatal(err)
	}
	if *reset {
		password, err := io.ReadAll(io.LimitReader(os.Stdin, 74))
		if err != nil {
			log.Fatal(err)
		}
		if err = admin.ResetPassword(strings.TrimRight(string(password), "\r\n")); err != nil {
			log.Fatal(err)
		}
		log.Println("Admin password updated; restart the service to apply it.")
		return
	}
	http.Handle("/admin/", admin)
	http.Handle("/v2/rooms", r)
	var s singleton
	http.HandleFunc("/v1/singleton", s.handler)
	log.Printf("preferans signaling listening on %s", *listen)
	server := http.Server{Addr: *listen, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	if *cert != "" || *key != "" {
		log.Fatal(server.ListenAndServeTLS(*cert, *key))
	}
	log.Fatal(server.ListenAndServe())
}
