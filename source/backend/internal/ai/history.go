package ai

// Riwayat percakapan HalloArus. PRINSIP PRIVASI (jangan dilonggarkan):
//   - Judul, isi pesan, dan kartu usulan disimpan TERENKRIPSI (AES-GCM, kunci terpisah dari API key, AAD = tenant:percakapan:seq).
//   - Hanya PEMILIK percakapan yang membaca/mengubah/menghapus: setiap query memfilter tenant_id DAN user_id (pemilik tenant tidak
//     otomatis melihat percakapan staf). Sesi "masuk-sebagai" ditolak.
//   - Foto nota dan hasil tool mentah tidak pernah disimpan (hanya penanda `had_images` dan nama tool).
//   - Retensi 90 hari (disembunyikan saat baca, dibersihkan saat pemilik membuat percakapan baru), maksimal 50 percakapan per pengguna
//     dan 200 pesan per percakapan. Pemilik tenant bisa mematikan penyimpanan (ai_prefs); "percakapan sementara" tidak pernah disimpan.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	historyDays      = 90
	maxConversations = 50
	maxConvMessages  = 200
	convTitleMax     = 60
	convListMax      = 50
)

var (
	ErrConvNotFound = errors.New("percakapan tidak ditemukan")
	ErrConvFull     = errors.New("percakapan sudah penuh")
	ErrHistoryOff   = errors.New("riwayat tidak aktif")
)

// WithHistory mengaktifkan penyimpanan riwayat dengan kunci enkripsi `key` (label khusus riwayat, terpisah dari API key).
func (s *Service) WithHistory(key string) (*Service, error) {
	b, err := newSecretBoxLabel(key, "aciraba-ai-history-v1")
	if err != nil {
		return s, err
	}
	s.hbox = b
	return s, nil
}

func aadTitle(tenant, conv uuid.UUID) string { return tenant.String() + ":" + conv.String() + ":title" }
func aadMsg(tenant, conv uuid.UUID, seq int, part string) string {
	return fmt.Sprintf("%s:%s:%d:%s", tenant, conv, seq, part)
}

// HistoryInfo dikirim ke UI: tersedia (kunci terpasang), menyala (saklar tenant), dan masa simpan.
type HistoryInfo struct {
	Available bool `json:"available"`
	Enabled   bool `json:"enabled"`
	Days      int  `json:"days"`
}

func (s *Service) historyInfo(ctx context.Context, a authz.Actor) HistoryInfo {
	h := HistoryInfo{Available: s.hbox != nil, Enabled: true, Days: historyDays}
	if !h.Available {
		h.Enabled = false
		return h
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT save_history FROM ai_prefs WHERE tenant_id = $1`, a.TenantID).Scan(&h.Enabled)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		h.Enabled = false // gagal membaca saklar → jangan menyimpan (tertutup)
	}
	if pol, perr := s.mustPolicy(ctx, a); perr != nil || !pol.AllowHistory {
		h.Enabled = false // platform melarang penyimpanan riwayat (menang atas saklar tenant)
	}
	return h
}

// SetHistoryEnabled = saklar simpan riwayat milik tenant (izin ai_assistant.update).
func (s *Service) SetHistoryEnabled(ctx context.Context, a authz.Actor, on bool) error {
	if a.Impersonator != uuid.Nil {
		return ErrImpersonating
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO ai_prefs (tenant_id, save_history, updated_by) VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id) DO UPDATE SET save_history = EXCLUDED.save_history, updated_by = EXCLUDED.updated_by, updated_at = now()`, a.TenantID, on, a.UserID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAISettings, Entity: audit.EntityAI, EntityID: a.TenantID.String(),
			Details: map[string]any{"history_enabled": on}})
	})
}

func convTitle(question string) string {
	t := strings.Join(strings.Fields(question), " ")
	r := []rune(t)
	if len(r) > convTitleMax {
		t = string(r[:convTitleMax-1]) + "…"
	}
	if t == "" {
		t = "Percakapan"
	}
	return t
}

// checkConversation memastikan percakapan milik pengguna, belum kedaluwarsa, dan masih muat satu giliran lagi.
func (s *Service) checkConversation(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var n int
		err := tx.QueryRow(ctx, `SELECT msg_count FROM ai_conversations WHERE tenant_id = $1 AND id = $2 AND user_id = $3
			AND updated_at > now() - make_interval(days => $4)`, a.TenantID, id, a.UserID, historyDays).Scan(&n)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConvNotFound
		}
		if err != nil {
			return err
		}
		if n+2 > maxConvMessages {
			return ErrConvFull
		}
		return nil
	})
}

// saveTurn menyimpan satu giliran (pertanyaan + jawaban) ke percakapan `convID` (nil = buat baru). Mengembalikan id percakapan.
func (s *Service) saveTurn(ctx context.Context, a authz.Actor, convID uuid.UUID, question string, hadImages bool, res *ChatResult) (uuid.UUID, error) {
	var out uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		id, base := convID, 0
		if id == uuid.Nil {
			// Bersihkan milik pengguna ini yang kedaluwarsa, lalu sisakan tempat (batas jumlah): yang terlama dibuang.
			if _, err := tx.Exec(ctx, `DELETE FROM ai_conversations WHERE tenant_id = $1 AND user_id = $2 AND updated_at <= now() - make_interval(days => $3)`, a.TenantID, a.UserID, historyDays); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM ai_conversations WHERE tenant_id = $1 AND id IN (
				SELECT id FROM ai_conversations WHERE tenant_id = $1 AND user_id = $2 ORDER BY updated_at DESC, id DESC OFFSET $3)`, a.TenantID, a.UserID, maxConversations-1); err != nil {
				return err
			}
			id = uuid.New()
			title, err := s.hbox.seal(convTitle(question), aadTitle(a.TenantID, id))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ai_conversations (id, tenant_id, user_id, title_enc) VALUES ($1, $2, $3, $4)`, id, a.TenantID, a.UserID, title); err != nil {
				return err
			}
		} else {
			err := tx.QueryRow(ctx, `SELECT msg_count FROM ai_conversations WHERE tenant_id = $1 AND id = $2 AND user_id = $3
				AND updated_at > now() - make_interval(days => $4) FOR UPDATE`, a.TenantID, id, a.UserID, historyDays).Scan(&base)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConvNotFound
			}
			if err != nil {
				return err
			}
			if base+2 > maxConvMessages {
				return ErrConvFull
			}
		}
		uEnc, err := s.hbox.seal(question, aadMsg(a.TenantID, id, base+1, "c"))
		if err != nil {
			return err
		}
		aEnc, err := s.hbox.seal(res.Reply, aadMsg(a.TenantID, id, base+2, "c"))
		if err != nil {
			return err
		}
		var actEnc *string
		if len(res.Actions) > 0 {
			raw, _ := json.Marshal(res.Actions)
			e, err := s.hbox.seal(string(raw), aadMsg(a.TenantID, id, base+2, "a"))
			if err != nil {
				return err
			}
			actEnc = &e
		}
		tools := make([]string, 0, len(res.ToolsUsed))
		seen := map[string]bool{}
		for _, c := range res.ToolsUsed {
			if !seen[c.Name] {
				seen[c.Name] = true
				tools = append(tools, c.Name)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ai_messages (conversation_id, tenant_id, seq, role, content_enc, had_images) VALUES ($1, $2, $3, 'user', $4, $5)`,
			id, a.TenantID, base+1, uEnc, hadImages); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ai_messages (conversation_id, tenant_id, seq, role, content_enc, actions_enc, tools) VALUES ($1, $2, $3, 'assistant', $4, $5, $6)`,
			id, a.TenantID, base+2, aEnc, actEnc, tools); err != nil {
			return err
		}
		for _, act := range res.Actions {
			aid, err := uuid.Parse(act.ID)
			if err != nil {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ai_actions (id, tenant_id, conversation_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, aid, a.TenantID, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE ai_conversations SET msg_count = $3, updated_at = now() WHERE tenant_id = $1 AND id = $2`, a.TenantID, id, base+2); err != nil {
			return err
		}
		out = id
		return nil
	})
	return out, err
}

// setActionStatus mencatat hasil kartu usulan (done/cancelled) untuk tampilan riwayat. Gagal tidak menggagalkan aksi.
func (s *Service) setActionStatus(ctx context.Context, a authz.Actor, actionID, status string) {
	id, err := uuid.Parse(actionID)
	if err != nil || s.hbox == nil { // tanpa riwayat tidak ada baris ai_actions untuk diperbarui
		return
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE ai_actions SET status = $3, updated_at = now() WHERE tenant_id = $1 AND id = $2`, a.TenantID, id, status)
		return err
	})
	if err != nil {
		s.log.WarnContext(ctx, "ai status aksi gagal", "err", err)
	}
}

// ---- Baca / kelola ----

type ConversationSummary struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	MsgCount  int       `json:"msg_count"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConversationPage struct {
	Items []ConversationSummary `json:"items"`
	Next  string                `json:"next,omitempty"` // kursor keyset; kosong = habis
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrBadInput
	}
	ts, ids, ok := strings.Cut(string(raw), "|")
	t, err1 := time.Parse(time.RFC3339Nano, ts)
	id, err2 := uuid.Parse(ids)
	if !ok || err1 != nil || err2 != nil {
		return time.Time{}, uuid.Nil, ErrBadInput
	}
	return t, id, nil
}

// ListConversations = percakapan milik pengguna, terbaru dulu (keyset, tanpa OFFSET).
func (s *Service) ListConversations(ctx context.Context, a authz.Actor, cursor string, limit int) (*ConversationPage, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrImpersonating
	}
	if s.hbox == nil {
		return &ConversationPage{Items: []ConversationSummary{}}, nil
	}
	limit = min(max(limit, 1), convListMax)
	var curT time.Time
	var curID uuid.UUID
	if cursor != "" {
		var err error
		if curT, curID, err = decodeCursor(cursor); err != nil {
			return nil, err
		}
	}
	page := &ConversationPage{Items: []ConversationSummary{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, title_enc, msg_count, updated_at FROM ai_conversations
			WHERE tenant_id = $1 AND user_id = $2 AND updated_at > now() - make_interval(days => $3)
			  AND ($4::timestamptz IS NULL OR (updated_at, id) < ($4, $5))
			ORDER BY updated_at DESC, id DESC LIMIT $6`, a.TenantID, a.UserID, historyDays, nullTime(curT), curID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c ConversationSummary
			var enc string
			if err := rows.Scan(&c.ID, &enc, &c.MsgCount, &c.UpdatedAt); err != nil {
				return err
			}
			if len(page.Items) == limit {
				page.Next = encodeCursor(page.Items[limit-1].UpdatedAt, page.Items[limit-1].ID)
				break
			}
			if c.Title, err = s.hbox.open(enc, aadTitle(a.TenantID, c.ID)); err != nil {
				c.Title = "…" // baris rusak/kunci berganti: jangan menggagalkan seluruh daftar
			}
			page.Items = append(page.Items, c)
		}
		return rows.Err()
	})
	return page, err
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// StoredAction = kartu usulan di riwayat + statusnya sekarang (pending|done|cancelled|expired).
type StoredAction struct {
	PendingAction
	Status string `json:"status"`
}

type StoredMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Tools     []string       `json:"tools,omitempty"`
	Actions   []StoredAction `json:"actions,omitempty"`
	HadImages bool           `json:"had_images,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Conversation struct {
	ConversationSummary
	Messages []StoredMessage `json:"messages"`
}

// GetConversation membuka satu percakapan milik pengguna.
func (s *Service) GetConversation(ctx context.Context, a authz.Actor, id uuid.UUID) (*Conversation, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrImpersonating
	}
	if s.hbox == nil {
		return nil, ErrConvNotFound
	}
	out := &Conversation{Messages: []StoredMessage{}}
	statuses := map[string]string{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var enc string
		err := tx.QueryRow(ctx, `SELECT id, title_enc, msg_count, updated_at FROM ai_conversations WHERE tenant_id = $1 AND id = $2 AND user_id = $3
			AND updated_at > now() - make_interval(days => $4)`, a.TenantID, id, a.UserID, historyDays).Scan(&out.ID, &enc, &out.MsgCount, &out.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConvNotFound
		}
		if err != nil {
			return err
		}
		if out.Title, err = s.hbox.open(enc, aadTitle(a.TenantID, id)); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT seq, role, content_enc, actions_enc, tools, had_images, created_at FROM ai_messages
			WHERE tenant_id = $1 AND conversation_id = $2 ORDER BY seq`, a.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var seq int
			var cEnc string
			var aEnc *string
			var m StoredMessage
			if err := rows.Scan(&seq, &m.Role, &cEnc, &aEnc, &m.Tools, &m.HadImages, &m.CreatedAt); err != nil {
				return err
			}
			if m.Content, err = s.hbox.open(cEnc, aadMsg(a.TenantID, id, seq, "c")); err != nil {
				return err
			}
			if aEnc != nil {
				raw, err := s.hbox.open(*aEnc, aadMsg(a.TenantID, id, seq, "a"))
				if err != nil {
					return err
				}
				var pas []PendingAction
				if json.Unmarshal([]byte(raw), &pas) == nil {
					for _, p := range pas {
						m.Actions = append(m.Actions, StoredAction{PendingAction: p})
					}
				}
			}
			out.Messages = append(out.Messages, m)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		ar, err := tx.Query(ctx, `SELECT id, status FROM ai_actions WHERE tenant_id = $1 AND conversation_id = $2`, a.TenantID, id)
		if err != nil {
			return err
		}
		defer ar.Close()
		for ar.Next() {
			var aid uuid.UUID
			var st string
			if err := ar.Scan(&aid, &st); err != nil {
				return err
			}
			statuses[aid.String()] = st
		}
		return ar.Err()
	})
	if err != nil {
		return nil, err
	}
	for i := range out.Messages {
		for j := range out.Messages[i].Actions {
			act := &out.Messages[i].Actions[j]
			act.Status = statuses[act.ID]
			if act.Status == "pending" || act.Status == "" {
				// Usulan yang belum diputuskan hanya hidup selama masih ada di Redis; setelah itu kedaluwarsa.
				if _, err := s.loadAction(ctx, a, act.ID); err == nil {
					act.Status = "pending"
				} else {
					act.Status = "expired"
				}
			}
		}
	}
	return out, nil
}

func (s *Service) RenameConversation(ctx context.Context, a authz.Actor, id uuid.UUID, title string) error {
	if a.Impersonator != uuid.Nil {
		return ErrImpersonating
	}
	if s.hbox == nil {
		return ErrConvNotFound
	}
	title = convTitle(title)
	enc, err := s.hbox.seal(title, aadTitle(a.TenantID, id))
	if err != nil {
		return err
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE ai_conversations SET title_enc = $4 WHERE tenant_id = $1 AND id = $2 AND user_id = $3`, a.TenantID, id, a.UserID, enc)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrConvNotFound
		}
		return nil
	})
}

func (s *Service) DeleteConversation(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	if a.Impersonator != uuid.Nil {
		return ErrImpersonating
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM ai_conversations WHERE tenant_id = $1 AND id = $2 AND user_id = $3`, a.TenantID, id, a.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrConvNotFound
		}
		return nil
	})
}

// DeleteAllConversations menghapus SEMUA riwayat milik pengguna (tidak menyentuh pengguna lain) dan mencatatnya di audit.
func (s *Service) DeleteAllConversations(ctx context.Context, a authz.Actor) (int64, error) {
	if a.Impersonator != uuid.Nil {
		return 0, ErrImpersonating
	}
	var n int64
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM ai_conversations WHERE tenant_id = $1 AND user_id = $2`, a.TenantID, a.UserID)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAIHistoryClear, Entity: audit.EntityAI, EntityID: a.UserID.String(),
			Details: map[string]any{"conversations": n}})
	})
	return n, err
}
