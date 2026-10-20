package api

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"pulsemargin/internal/checkout"
)

//go:embed static
var staticFS embed.FS

type handler struct {
	scorer *checkout.Scorer
}

// NewHandler builds the API routes.
func NewHandler(scorer *checkout.Scorer) http.Handler {
	h := &handler{scorer: scorer}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /api/v1/checkout/score", h.checkoutScore)

	staticDir, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticDir))))
	mux.HandleFunc("GET /{$}", h.index)

	return mux
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	page, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

func (h *handler) checkoutScore(w http.ResponseWriter, r *http.Request) {
	var payload checkout.Payload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON payload")
		return
	}

	if payload.CheckoutID == "" || payload.CustomerID == "" || len(payload.Items) == 0 {
		writeError(w, http.StatusBadRequest, "Missing required fields (checkout_id, customer_id, items)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()

	decision, err := h.scorer.Score(ctx, payload)
	if err != nil {
		log.Printf("Checkout scoring failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Scoring failed")
		return
	}

	writeJSON(w, http.StatusOK, decision)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
