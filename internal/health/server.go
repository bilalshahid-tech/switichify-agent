package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

type Server struct {
	port       string
	running    int32
	lastPingOk int32
}

func New(port string) *Server {
	return &Server{port: port}
}

func (s *Server) SetRunning(ok bool) {
	if ok {
		atomic.StoreInt32(&s.running, 1)
	} else {
		atomic.StoreInt32(&s.running, 0)
	}
}

func (s *Server) SetPingHealthy(ok bool) {
	if ok {
		atomic.StoreInt32(&s.lastPingOk, 1)
	} else {
		atomic.StoreInt32(&s.lastPingOk, 0)
	}
}

func (s *Server) Serve() error {
	http.HandleFunc("/health", s.handleHealth)
	return http.ListenAndServe("127.0.0.1:"+s.port, nil)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"running": atomic.LoadInt32(&s.running) == 1,
		"ping_ok": atomic.LoadInt32(&s.lastPingOk) == 1,
	}
	json.NewEncoder(w).Encode(resp)
}
