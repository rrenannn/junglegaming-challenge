package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write JSON response", "error", err)
	}
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorBody{Error: code, Message: message})
}
