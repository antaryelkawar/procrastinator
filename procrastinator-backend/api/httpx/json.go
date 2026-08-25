package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON marshals v to JSON and writes it to w with the given HTTP status.
// Content-Type is set to application/json. No trailing newline is appended.
// If marshaling fails, a 500 response with a fixed error body is written instead.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")

	data, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to encode response"}`))
		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// WriteError writes a JSON error envelope {"error":"msg"} with the given status.
// It delegates to WriteJSON to guarantee a consistent envelope shape.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}
