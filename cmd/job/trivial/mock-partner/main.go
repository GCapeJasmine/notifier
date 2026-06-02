package main

import (
	"encoding/json"
	"io"
	"math/rand"
	"net/http"

	"go.uber.org/zap"

	"github.com/gleo/subscribers/common/log"
)

var errorCodes = []int{
	http.StatusBadRequest,          // 400
	http.StatusTooManyRequests,     // 429
	http.StatusInternalServerError, // 500
	http.StatusBadGateway,          // 502
	http.StatusServiceUnavailable,  // 503
}

func respond(w http.ResponseWriter) int {
	if rand.Intn(100) < 80 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return http.StatusOK
	}
	code := errorCodes[rand.Intn(len(errorCodes))]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write([]byte(`{"status":"error"}`))
	return code
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			payload = string(body)
		}

		code := respond(w)

		log.Logger.Infow("mock-partner: responded",
			"status", code,
			"method", r.Method,
			"path", r.URL.Path,
			zap.Any("payload", payload),
		)
	})

	log.Logger.Infow("mock-partner: listening on :8080 (80% OK / 20% error)")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Logger.Fatalw("mock-partner: server error", zap.Error(err))
	}
}
