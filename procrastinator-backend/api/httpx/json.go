package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteErrorEnvelope writes the {"error": msg} JSON envelope with the given
// status and Content-Type: application/json. It is the single shared writer
// for the error envelope (used by the api package's centralized error funcs
// and by UserMiddleware's pre-handler failures).
func WriteErrorEnvelope(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(body)
}
