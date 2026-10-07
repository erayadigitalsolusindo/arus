package httpx

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimit membatasi `limit` request per `window` per IP klien (fixed window di Redis).
// `name` memisahkan counter antar endpoint. Bila Redis gagal, request ditolak (fail closed).
func RateLimit(rdb *redis.Client, name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			key := fmt.Sprintf("rl:%s:%s", name, ip)

			pipe := rdb.TxPipeline()
			incr := pipe.Incr(r.Context(), key)
			pipe.ExpireNX(r.Context(), key, window)
			if _, err := pipe.Exec(r.Context()); err != nil {
				Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
				return
			}
			if incr.Val() > int64(limit) {
				ttl, _ := rdb.TTL(r.Context(), key).Result()
				if ttl < time.Second {
					ttl = window
				}
				w.Header().Set("Retry-After", strconv.Itoa(int(ttl.Seconds())))
				Error(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan. Coba lagi nanti.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
