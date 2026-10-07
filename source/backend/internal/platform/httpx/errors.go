// Package httpx berisi router, middleware, dan format respons HTTP.
package httpx

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Fields: kode galat per field (mis. {"email":"INVALID"}) agar klien menerjemahkannya sendiri.
	Fields map[string]string `json:"fields,omitempty"`
	// RetryAfter: detik sampai boleh mencoba lagi (RATE_LIMITED, ACCOUNT_LOCKED).
	RetryAfter int `json:"retry_after,omitempty"`
	// AttemptsLeft: sisa percobaan login gagal sebelum akun dikunci (INVALID_CREDENTIALS).
	AttemptsLeft int `json:"attempts_left,omitempty"`
}

// JSON menulis respons JSON dengan status yang diberikan.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error menulis error API: {"error":{"code","message"}} sesuai AGENTS.md §6.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// ErrorAttempts = Error + sisa percobaan sebelum dikunci.
func ErrorAttempts(w http.ResponseWriter, status int, code, message string, attemptsLeft int) {
	JSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, AttemptsLeft: attemptsLeft}})
}

// ValidationError menulis 422 VALIDATION dengan kode galat per field.
func ValidationError(w http.ResponseWriter, fields map[string]string) {
	JSON(w, http.StatusUnprocessableEntity, errorBody{Error: errorDetail{
		Code:    "VALIDATION",
		Message: "Data yang dikirim tidak valid.",
		Fields:  fields,
	}})
}
