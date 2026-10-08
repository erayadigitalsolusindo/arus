package item

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

// PriceReportModuleID = modul izin laporan Riwayat Harga Jual lintas item (authz.Modules).
const PriceReportModuleID = "sell_price_history"

const (
	reportDefLimit = 50
	reportMaxLimit = 200
	reportMaxDays  = 366
)

// PriceReportEvent = satu kejadian perubahan harga + barang yang berubah (nama/kode saat kejadian).
type PriceReportEvent struct {
	PriceEvent
	ItemID uuid.UUID `json:"item_id"`
	SKU    string    `json:"sku"`
	Name   string    `json:"name"`
}

type PriceReport struct {
	From       string             `json:"from"`
	To         string             `json:"to"`
	Items      []PriceReportEvent `json:"items"`
	NextCursor string             `json:"next_cursor"`
}

// PriceReportParams: From/To = YYYY-MM-DD menurut zona waktu outlet aktif (kosong = 30 hari terakhir).
// Outlet = "" (semua), "default" (harga default) atau id cabang yang boleh diakses pemanggil.
type PriceReportParams struct {
	From, To string
	Q        string
	Outlet   string
	Cursor   string
	Limit    int
}

func encodeReportCursor(at time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + ":" + strconv.FormatInt(id, 10)))
}

func decodeReportCursor(c string) (time.Time, int64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, 0, false
	}
	micro, idStr, ok := strings.Cut(string(raw), ":")
	us, err1 := strconv.ParseInt(micro, 10, 64)
	id, err2 := strconv.ParseInt(idStr, 10, 64)
	if !ok || err1 != nil || err2 != nil {
		return time.Time{}, 0, false
	}
	return time.UnixMicro(us).UTC(), id, true
}

func parseReportDay(s string) (pgtype.Date, bool) {
	if s == "" {
		return pgtype.Date{}, true
	}
	d, err := time.Parse("2006-01-02", s)
	return pgtype.Date{Time: d, Valid: err == nil}, err == nil
}

// PriceReport mengembalikan perubahan harga jual semua barang pada periode, terbaru dulu.
// Perubahan pada cabang di luar akses pemanggil disaring; halaman bisa lebih pendek dari limit (kursor tetap lanjut).
func (s *Service) PriceReport(ctx context.Context, a authz.Actor, p PriceReportParams) (*PriceReport, error) {
	fe := FieldErrors{}
	from, okFrom := parseReportDay(p.From)
	to, okTo := parseReportDay(p.To)
	if !okFrom {
		fe["from"] = sanitize.Invalid
	}
	if !okTo {
		fe["to"] = sanitize.Invalid
	}
	q := strings.TrimSpace(p.Q)
	if len([]rune(q)) > 100 {
		fe["q"] = sanitize.TooLong
	}
	if p.Outlet != "" && p.Outlet != "default" {
		if oid, err := uuid.Parse(p.Outlet); err != nil || !a.Outlets[oid] {
			fe["outlet"] = sanitize.Invalid
		}
	}
	var cursorAt pgtype.Timestamptz
	var cursorID pgtype.Int8
	if p.Cursor != "" {
		at, id, ok := decodeReportCursor(p.Cursor)
		if !ok {
			fe["cursor"] = sanitize.Invalid
		}
		cursorAt, cursorID = pgtype.Timestamptz{Time: at, Valid: ok}, pgtype.Int8{Int64: id, Valid: ok}
	}
	if len(fe) > 0 {
		return nil, fe
	}
	limit := p.Limit
	if limit <= 0 {
		limit = reportDefLimit
	}
	limit = min(limit, reportMaxLimit)

	out := &PriceReport{Items: []PriceReportEvent{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var fromDay, toDay time.Time
		var startAt, endAt time.Time
		err := tx.QueryRow(ctx, `
			WITH o AS (
			    SELECT timezone, (now() AT TIME ZONE timezone)::date AS today FROM outlets WHERE tenant_id = $1 AND id = $2
			), t AS (SELECT o.timezone, coalesce($4::date, o.today) AS to_day FROM o),
			d AS (SELECT t.timezone, t.to_day, coalesce($3::date, t.to_day - 29) AS from_day FROM t)
			SELECT d.from_day::date, d.to_day::date,
			       (d.from_day::timestamp AT TIME ZONE d.timezone)::timestamptz,
			       ((d.to_day + 1)::timestamp AT TIME ZONE d.timezone)::timestamptz
			FROM d`, a.TenantID, a.OutletID, from, to).Scan(&fromDay, &toDay, &startAt, &endAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return FieldErrors{"from": sanitize.Invalid}
		}
		if err != nil {
			return err
		}
		if toDay.Before(fromDay) || toDay.Sub(fromDay) > reportMaxDays*24*time.Hour {
			return FieldErrors{"to": sanitize.Invalid}
		}
		out.From, out.To = fromDay.Format("2006-01-02"), toDay.Format("2006-01-02")

		pattern := ""
		if q != "" {
			pattern = "%" + likeEscape(q) + "%"
		}
		rows, err := tx.Query(ctx, `
			SELECT id, entity_id, actor_name, action, details, created_at
			FROM audit_log
			WHERE tenant_id = $1 AND entity = $2 AND action IN ($3, $4)
			  AND created_at >= $5 AND created_at < $6
			  AND ($7::text = '' OR details->>'sku' ILIKE $7 OR details->>'name' ILIKE $7)
			  AND ($8::text = '' OR (action = $4 AND $8 = 'default')
			       OR details->'before' ? $8 OR details->'after' ? $8
			       OR details->'wholesale_before' ? $8 OR details->'wholesale_after' ? $8)
			  AND ($9::timestamptz IS NULL OR (created_at, id) < ($9::timestamptz, $10::bigint))
			ORDER BY created_at DESC, id DESC
			LIMIT $11`,
			a.TenantID, audit.EntityItem, audit.ActionItemPrice, audit.ActionItemCreate, startAt, endAt, pattern, p.Outlet, cursorAt, cursorID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		n := 0
		var lastAt time.Time
		var lastID int64
		for rows.Next() {
			var (
				id       int64
				entityID string
				actor    string
				action   string
				raw      []byte
				at       time.Time
			)
			if err := rows.Scan(&id, &entityID, &actor, &action, &raw, &at); err != nil {
				return err
			}
			n++
			if n > limit {
				out.NextCursor = encodeReportCursor(lastAt, lastID)
				break
			}
			lastAt, lastID = at, id
			itemID, perr := uuid.Parse(entityID)
			if perr != nil {
				continue
			}
			details := map[string]any{}
			_ = json.Unmarshal(raw, &details)
			e := audit.Item{ID: id, ActorName: actor, Action: action, Details: details, CreatedAt: at}
			var ev PriceEvent
			ok := false
			if action == audit.ActionItemCreate {
				if v, isStr := details["sell_price"].(string); isStr {
					ev = PriceEvent{ID: id, At: at, ActorName: actor, Kind: HistoryInitial, Changes: []PriceChange{{After: &v}}}
					ok = true
				}
			} else {
				ev, ok = priceEvent(e, a)
			}
			if !ok {
				continue
			}
			sku, _ := details["sku"].(string)
			name, _ := details["name"].(string)
			out.Items = append(out.Items, PriceReportEvent{PriceEvent: ev, ItemID: itemID, SKU: sku, Name: name})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PriceReport: GET /items/price-history?from=&to=&q=&outlet=&cursor=&limit=
func (h *Handler) PriceReport(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.ActorFrom(r.Context())
	qv := r.URL.Query()
	p := PriceReportParams{From: qv.Get("from"), To: qv.Get("to"), Q: qv.Get("q"), Outlet: qv.Get("outlet"), Cursor: qv.Get("cursor")}
	if v := qv.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httpx.ValidationError(w, map[string]string{"limit": sanitize.Invalid})
			return
		}
		p.Limit = n
	}
	res, err := h.svc.PriceReport(r.Context(), actor, p)
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case err != nil:
		h.log.Error("laporan harga jual gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}
