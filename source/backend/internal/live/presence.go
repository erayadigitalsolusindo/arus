package live

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	presenceKey = "live:presence"
	touchEvery  = 30 * time.Second // satu tulis Redis per pengguna per 30 dtk, bukan per request
	// OnlineWindow = pengguna dianggap online bila terlihat aktif dalam rentang ini.
	OnlineWindow = 5 * time.Minute
	maxOnline    = 300
)

// Presence mencatat pengguna tenant yang baru saja memakai API (ZSET Redis: "tenant:user" → detik terakhir terlihat).
type Presence struct {
	rdb  *redis.Client
	mu   sync.Mutex
	last map[[2]uuid.UUID]time.Time
}

func NewPresence(rdb *redis.Client) *Presence {
	return &Presence{rdb: rdb, last: map[[2]uuid.UUID]time.Time{}}
}

// Touch dipasang sebagai authz.Resolver.WithPresence. Tidak memblokir request: tulis Redis di goroutine, gagal diabaikan.
func (p *Presence) Touch(tenant, user uuid.UUID) {
	if p == nil || p.rdb == nil {
		return
	}
	k, now := [2]uuid.UUID{tenant, user}, time.Now()
	p.mu.Lock()
	if now.Sub(p.last[k]) < touchEvery {
		p.mu.Unlock()
		return
	}
	if len(p.last) > 50_000 { // buang yang sudah lewat jendela online
		for kk, t := range p.last {
			if now.Sub(t) > OnlineWindow {
				delete(p.last, kk)
			}
		}
	}
	p.last[k] = now
	p.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pipe := p.rdb.Pipeline()
		pipe.ZAdd(ctx, presenceKey, redis.Z{Score: float64(now.Unix()), Member: tenant.String() + ":" + user.String()})
		pipe.ZRemRangeByScore(ctx, presenceKey, "-inf", "("+itoa(now.Add(-time.Hour).Unix()))
		_, _ = pipe.Exec(ctx)
	}()
}

type Online struct {
	Tenant, User uuid.UUID
	Seen         time.Time
}

// Online = pengguna yang terlihat aktif dalam OnlineWindow, terbaru dulu (maks 300).
func (p *Presence) Online(ctx context.Context) ([]Online, error) {
	zs, err := p.rdb.ZRevRangeByScoreWithScores(ctx, presenceKey, &redis.ZRangeBy{
		Min: itoa(time.Now().Add(-OnlineWindow).Unix()), Max: "+inf", Count: maxOnline}).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Online, 0, len(zs))
	for _, z := range zs {
		m, _ := z.Member.(string)
		a, b, ok := strings.Cut(m, ":")
		t, e1 := uuid.Parse(a)
		u, e2 := uuid.Parse(b)
		if !ok || e1 != nil || e2 != nil {
			continue
		}
		out = append(out, Online{Tenant: t, User: u, Seen: time.Unix(int64(z.Score), 0)})
	}
	return out, nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
