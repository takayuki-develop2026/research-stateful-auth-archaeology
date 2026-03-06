package adminapi

import "net/http"

func RegisterV17MobileRoutes(mux *http.ServeMux, h *V17MobileHandler, dev *V17MobileDevHandler) {
	mux.HandleFunc("POST /v17/mobile/devices/register", h.RegisterDevice)
	mux.HandleFunc("POST /v17/mobile/stepup/request", h.RequestStepUp)
	mux.HandleFunc("POST /v17/mobile/stepup/verify", h.VerifyStepUp)
	mux.HandleFunc("GET /v17/mobile/inbox", h.ListInbox)

	if dev != nil {
		mux.HandleFunc("POST /v17/mobile/dev/seed/inbox", dev.SeedInbox)
	}

	mux.HandleFunc("POST /v17/mobile/inbox/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case hasSuffix(r.URL.Path, "/ack"):
			h.AckInboxItem(w, r)
			return
		case hasSuffix(r.URL.Path, "/approve"):
			h.ApproveInboxItem(w, r)
			return
		case hasSuffix(r.URL.Path, "/reject"):
			h.RejectInboxItem(w, r)
			return
		default:
			writeError(w, http.StatusNotFound, "not_found", "route not found", ensureTraceIDFromHeader(r))
			return
		}
	})
}

func hasSuffix(v, s string) bool {
	if len(v) < len(s) {
		return false
	}
	return v[len(v)-len(s):] == s
}