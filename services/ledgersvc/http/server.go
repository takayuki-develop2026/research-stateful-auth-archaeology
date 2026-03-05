package httpapi

import (
	"net/http"
	"time"
)

type Server struct {
	Addr    string
	Handler http.Handler
}

func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           s.Handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}