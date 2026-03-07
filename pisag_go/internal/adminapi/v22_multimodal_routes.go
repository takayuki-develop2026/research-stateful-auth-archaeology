package adminapi

import (
	"net/http"
	"strings"
)

func RegisterV22MultimodalRoutes(mux *http.ServeMux, h *V22MultimodalHandler) {
	mux.HandleFunc("/v22/multimodal/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.CreateTask(w, r)
		case http.MethodGet:
			h.ListTasks(w, r)
		default:
			v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodPost, http.MethodGet)
		}
	})

	mux.HandleFunc("/v22/multimodal/tasks/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/execute/ocr"):
			if r.Method != http.MethodPost {
				v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodPost)
				return
			}
			h.ExecuteOCRTask(w, r)
			return
		case strings.HasSuffix(r.URL.Path, "/execute/vision"):
			if r.Method != http.MethodPost {
				v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodPost)
				return
			}
			h.ExecuteVisionTask(w, r)
			return
		default:
			if r.Method != http.MethodGet {
				v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodGet)
				return
			}
			h.GetTask(w, r)
		}
	})

	mux.HandleFunc("/v22/multimodal/results", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodGet)
			return
		}
		h.ListResults(w, r)
	})

	mux.HandleFunc("/v22/multimodal/results/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodGet)
			return
		}
		h.GetResult(w, r)
	})

	mux.HandleFunc("/v22/multimodal/review-queue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			v22MethodNotAllowed(w, v22EnsureTraceID(r), r.Method, http.MethodGet)
			return
		}
		h.ListReviewQueue(w, r)
	})
}
