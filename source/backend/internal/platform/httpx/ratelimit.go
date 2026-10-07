package httpx

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Allow menghitung satu percobaan pada `key` (fixed window di Redis). Mengembalikan false bila batas
// terlampaui, beserta sisa waktu tunggu. Error = Redis gagal (pemanggil sebaiknya menolak: fail closed).
func Allow(ctx context.Context, rdb *redis.Client, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	pipe := rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, err
	}
	if incr.Val() <= int64(limit) {
		return true, 0, nil
	}
	ttl, _ := rdb.TTL(ctx, key).Result()
	if ttl < time.Second {
		ttl = window
	}
	return false, ttl, nil
}

// TooManyRequests menulis 429 RATE_LIMITED dengan header Retry-After.
func TooManyRequests(w http.ResponseWriter, retryAfter time.Duration) {
	Retry(w, "RATE_LIMITED", "Terlalu banyak percobaan. Coba lagi nanti.", retryAfter)
}

// Retry menulis 429 dengan `code`, header Retry-After, dan retry_after (detik) di body agar klien bisa menampilkan hitung mundur.
func Retry(w http.ResponseWriter, code, message string, retryAfter time.Duration) {
	secs := int(math.Ceil(retryAfter.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	JSON(w, http.StatusTooManyRequests, errorBody{Error: errorDetail{Code: code, Message: message, RetryAfter: secs}})
}

// ClientIP = IP peer (sudah diganti RealIP bila TRUST_PROXY aktif).
func ClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RateLimit membatasi `limit` request per `window` per IP klien. `name` memisahkan counter antar endpoint.
// Bila Redis gagal, request ditolak (fail closed).
func RateLimit(rdb *redis.Client, name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, wait, err := Allow(r.Context(), rdb, fmt.Sprintf("rl:%s:%s", name, ClientIP(r)), limit, window)
			switch {
			case err != nil:
				Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
			case !ok:
				TooManyRequests(w, wait)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}
