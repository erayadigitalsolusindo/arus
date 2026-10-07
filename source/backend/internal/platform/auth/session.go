package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Umur refresh token. "Tetap masuk" = panjang; selain itu sesi pendek.
const (
	RefreshTTLRemember = 30 * 24 * time.Hour
	RefreshTTLShort    = 12 * time.Hour

	// RotateGrace = jendela setelah rotasi di mana token lama masih diterima untuk menerbitkan token akses
	// (tanpa rotasi lagi). Menutup balapan dua tab/permintaan yang me-refresh bersamaan dengan cookie yang sama.
	RotateGrace = 10 * time.Second
)

// Sessions menyimpan refresh token di Redis. Hanya hash SHA-256 token yang menjadi key, sehingga
// kebocoran isi Redis tidak membocorkan token yang masih berlaku.
//
// Skema key:
//
//	refresh:<hash>  "uid|family|r"        token aktif (r = 1 bila "tetap masuk")
//	used:<hash>     "family|uid|r|unix"   token yang sudah dirotasi (untuk grace & deteksi pemakaian ulang)
//	fam:<family>    "refresh:<hash>"      token aktif terkini dalam satu rantai rotasi
//	usr:<uid>       set of family         seluruh rantai milik satu pengguna (untuk pencabutan massal)
//	fo:<family>     outlet id             outlet aktif sesi ini (hasil "pindah outlet"), bertahan lintas refresh
//
// Pemakaian ulang token lama di luar jendela grace dianggap pencurian: seluruh rantai dicabut.
type Sessions struct{ rdb *redis.Client }

func NewSessions(rdb *redis.Client) *Sessions { return &Sessions{rdb: rdb} }

func hashOf(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func ttlFor(remember bool) time.Duration {
	if remember {
		return RefreshTTLRemember
	}
	return RefreshTTLShort
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token acak: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Create membuat refresh token baru (rantai rotasi baru) untuk userID.
func (s *Sessions) Create(ctx context.Context, userID string, remember bool) (string, error) {
	tok, err := newToken()
	if err != nil {
		return "", err
	}
	fam, err := newToken()
	if err != nil {
		return "", err
	}
	ttl := ttlFor(remember)
	r := "0"
	if remember {
		r = "1"
	}
	key := "refresh:" + hashOf(tok)
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, key, userID+"|"+fam+"|"+r, ttl)
	pipe.Set(ctx, "fam:"+fam, key, ttl)
	pipe.SAdd(ctx, "usr:"+userID, fam)
	pipe.Expire(ctx, "usr:"+userID, RefreshTTLRemember)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("simpan sesi: %w", err)
	}
	return tok, nil
}

// RotateStatus = hasil Rotate.
type RotateStatus int

const (
	RotateInvalid RotateStatus = iota // token tidak dikenal/kedaluwarsa
	RotateOK                          // token lama dicabut, token baru diterbitkan
	RotateGraceOK                     // token lama baru saja dirotasi: boleh dapat token akses, tanpa token baru
	RotateReuse                       // pemakaian ulang di luar grace: rantai dicabut
)

type RotateResult struct {
	Status   RotateStatus
	UserID   string
	Family   string // id rantai sesi (untuk preferensi outlet)
	Token    string // token refresh baru; kosong kecuali Status == RotateOK
	Remember bool
}

// Atomik: baca-cabut-terbitkan dalam satu eval agar dua permintaan bersamaan tidak sama-sama menang.
var rotateScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if v then
  local uid, fam, r = string.match(v, '^([^|]+)|([^|]+)|([01])$')
  local ttl = (r == '1') and ARGV[1] or ARGV[2]
  redis.call('DEL', KEYS[1])
  redis.call('SET', KEYS[3], v, 'EX', ttl)
  redis.call('SET', 'fam:' .. fam, KEYS[3], 'EX', ttl)
  redis.call('EXPIRE', 'fo:' .. fam, ttl)
  redis.call('SET', KEYS[2], fam .. '|' .. uid .. '|' .. r .. '|' .. ARGV[4], 'EX', ARGV[1])
  return {1, uid, r, fam}
end
local u = redis.call('GET', KEYS[2])
if u then
  local fam, uid, r, ts = string.match(u, '^([^|]+)|([^|]+)|([01])|(%d+)$')
  -- Rantai yang sudah dicabut (logout/reset password) tidak boleh hidup lagi lewat jendela grace.
  if redis.call('EXISTS', 'fam:' .. fam) == 0 then return {0} end
  if tonumber(ARGV[4]) - tonumber(ts) <= tonumber(ARGV[3]) then
    return {2, uid, r, fam}
  end
  local cur = redis.call('GET', 'fam:' .. fam)
  if cur then redis.call('DEL', cur) end
  redis.call('DEL', 'fam:' .. fam)
  redis.call('DEL', 'fo:' .. fam)
  return {3, uid, r, fam}
end
return {0}
`)

// Rotate menukar refresh token lama dengan yang baru.
func (s *Sessions) Rotate(ctx context.Context, old string) (RotateResult, error) {
	fresh, err := newToken()
	if err != nil {
		return RotateResult{}, err
	}
	h := hashOf(old)
	res, err := rotateScript.Run(ctx, s.rdb,
		[]string{"refresh:" + h, "used:" + h, "refresh:" + hashOf(fresh)},
		int(RefreshTTLRemember.Seconds()), int(RefreshTTLShort.Seconds()), int(RotateGrace.Seconds()), time.Now().Unix(),
	).Slice()
	if err != nil {
		return RotateResult{}, fmt.Errorf("rotasi sesi: %w", err)
	}
	code, _ := res[0].(int64)
	if code == 0 || len(res) < 4 {
		return RotateResult{Status: RotateInvalid}, nil
	}
	uid, _ := res[1].(string)
	rem, _ := res[2].(string)
	fam, _ := res[3].(string)
	out := RotateResult{Status: RotateStatus(code), UserID: uid, Remember: rem == "1", Family: fam}
	if out.Status == RotateOK {
		out.Token = fresh
	}
	return out, nil
}

// FamilyOf mengembalikan (userID, family) dari refresh token aktif; ok=false bila tidak dikenal.
func (s *Sessions) FamilyOf(ctx context.Context, token string) (userID, family string, ok bool, err error) {
	v, err := s.rdb.Get(ctx, "refresh:"+hashOf(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("baca sesi: %w", err)
	}
	parts := strings.Split(v, "|")
	if len(parts) != 3 {
		return "", "", false, nil
	}
	return parts[0], parts[1], true, nil
}

// SetOutlet menyimpan outlet aktif untuk satu sesi (rantai); dipakai Refresh agar pilihan outlet tidak hilang.
func (s *Sessions) SetOutlet(ctx context.Context, family, outletID string) error {
	if err := s.rdb.Set(ctx, "fo:"+family, outletID, RefreshTTLRemember).Err(); err != nil {
		return fmt.Errorf("simpan outlet sesi: %w", err)
	}
	return nil
}

// Outlet mengembalikan outlet pilihan sesi, atau "" bila belum pernah memilih.
func (s *Sessions) Outlet(ctx context.Context, family string) (string, error) {
	v, err := s.rdb.Get(ctx, "fo:"+family).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("baca outlet sesi: %w", err)
	}
	return v, nil
}

// Revoke mencabut seluruh rantai rotasi yang berisi token ini (logout). Token tak dikenal diabaikan.
func (s *Sessions) Revoke(ctx context.Context, token string) error {
	_, fam, ok, err := s.FamilyOf(ctx, token)
	if err != nil || !ok {
		return err
	}
	if err := revokeFamily.Run(ctx, s.rdb, nil, fam).Err(); err != nil {
		return fmt.Errorf("cabut sesi: %w", err)
	}
	return nil
}

var revokeFamily = redis.NewScript(`
local cur = redis.call('GET', 'fam:' .. ARGV[1])
if cur then redis.call('DEL', cur) end
redis.call('DEL', 'fam:' .. ARGV[1])
redis.call('DEL', 'fo:' .. ARGV[1])
return 1
`)

var revokeUser = redis.NewScript(`
local fams = redis.call('SMEMBERS', 'usr:' .. ARGV[1])
for _, fam in ipairs(fams) do
  local cur = redis.call('GET', 'fam:' .. fam)
  if cur then redis.call('DEL', cur) end
  redis.call('DEL', 'fam:' .. fam)
  redis.call('DEL', 'fo:' .. fam)
end
redis.call('DEL', 'usr:' .. ARGV[1])
return #fams
`)

// RevokeUser mencabut SEMUA sesi refresh milik satu pengguna di semua perangkat (reset/ganti password, penonaktifan).
// Token akses yang masih hidup dicabut terpisah lewat users.tokens_valid_after.
func (s *Sessions) RevokeUser(ctx context.Context, userID string) error {
	if err := revokeUser.Run(ctx, s.rdb, nil, userID).Err(); err != nil {
		return fmt.Errorf("cabut semua sesi: %w", err)
	}
	return nil
}
