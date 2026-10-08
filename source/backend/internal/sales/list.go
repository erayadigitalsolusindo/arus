package sales

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

const (
	listLimit   = 500
	maxListDays = 62
)

// ListRow = ringkasan satu nota pada daftar penjualan kasir.
type ListRow struct {
	ID        uuid.UUID         `json:"id"`
	DocNo     string            `json:"doc_no"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	Cashier   string            `json:"cashier"`
	Member    string            `json:"member,omitempty"`
	LineCount int               `json:"line_count"`
	Total     string            `json:"total"`
	Methods   map[string]string `json:"methods"` // jenis → jumlah (tunai sudah bersih dari kembalian)
	Pays      []MethodAmount    `json:"pays"`    // per metode (id + nama sekarang)
}

// MethodAmount = jumlah per metode pembayaran (tunai bersih dari kembalian).
type MethodAmount struct {
	MethodID uuid.UUID `json:"method_id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Amount   string    `json:"amount"`
}

// ListResult: Totals = jumlah per metode atas SELURUH baris yang dikembalikan; Truncated bila terpotong batas.
type ListResult struct {
	Data      []ListRow         `json:"data"`
	Total     string            `json:"total"`
	Totals    map[string]string `json:"totals"`    // per jenis
	ByMethod  []MethodAmount    `json:"by_method"` // per metode: Tunai dulu, lalu menurut nama
	From      string            `json:"from"`
	To        string            `json:"to"`
	Truncated bool              `json:"truncated"`
}

// List = nota outlet aktif milik KASIR YANG SEDANG LOGIN (untuk mencocokkan uang fisik di lacinya; kasir lain tak terlihat) pada rentang tanggal (zona waktu outlet; kosong = hari ini), terbaru dulu, opsional cari no. nota.
func (s *Service) List(ctx context.Context, a authz.Actor, from, to, q string) (ListResult, error) {
	res := ListResult{Data: []ListRow{}, Totals: map[string]string{}, ByMethod: []MethodAmount{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		qr := gen.New(tx)
		o, err := qr.SalesOutletInfo(ctx, gen.SalesOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		today := o.LocalDay.Time
		fd, td := today, today
		fe := FieldErrors{}
		if from != "" {
			if fd, err = time.Parse("2006-01-02", from); err != nil {
				fe["from"] = "INVALID"
			}
		}
		if to != "" {
			if td, err = time.Parse("2006-01-02", to); err != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && (td.Before(fd) || td.Sub(fd) > maxListDays*24*time.Hour) {
			fe["to"] = "INVALID"
		}
		if len(fe) > 0 {
			return fe
		}
		q = strings.TrimSpace(q)
		if len(q) > 60 {
			q = q[:60]
		}
		rows, err := qr.SalesList(ctx, gen.SalesListParams{TenantID: a.TenantID, OutletID: a.OutletID, CashierID: pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}, AllCashiers: a.Impersonator != uuid.Nil, FromDay: pgtype.Date{Time: fd, Valid: true},
			ToDay: pgtype.Date{Time: td, Valid: true}, Q: q})
		if err != nil {
			return err
		}
		res.From, res.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
		res.Truncated = len(rows) >= listLimit
		sums := map[string]decimal.Decimal{}
		perMethod := map[uuid.UUID]*MethodAmount{}
		perAmt := map[uuid.UUID]decimal.Decimal{}
		var order []uuid.UUID
		all := decimal.Zero
		for _, r := range rows {
			if r.Status != "completed" {
				// Nota batal tetap terlihat di daftar tapi tidak masuk hitungan uang (laci kasir).
				res.Data = append(res.Data, ListRow{ID: r.ID, DocNo: r.DocNo, Status: r.Status, CreatedAt: r.CreatedAt.Time, Cashier: r.CashierName,
					Member: r.MemberName, LineCount: int(r.LineCount), Total: r.Total.StringFixed(2), Methods: map[string]string{}, Pays: []MethodAmount{}})
				continue
			}
			methods := map[string]string{}
			pays := []MethodAmount{}
			var parts []struct {
				ID     uuid.UUID       `json:"id"`
				Name   string          `json:"name"`
				Kind   string          `json:"kind"`
				Amount decimal.Decimal `json:"amount"`
			}
			if err := json.Unmarshal([]byte(r.PayAmounts), &parts); err != nil {
				return err
			}
			for _, p := range parts {
				prev, _ := decimal.NewFromString(methods[p.Kind])
				methods[p.Kind] = prev.Add(p.Amount).StringFixed(2)
				sums[p.Kind] = sums[p.Kind].Add(p.Amount)
				pays = append(pays, MethodAmount{MethodID: p.ID, Name: p.Name, Kind: p.Kind, Amount: p.Amount.StringFixed(2)})
				if _, ok := perMethod[p.ID]; !ok {
					perMethod[p.ID] = &MethodAmount{MethodID: p.ID, Name: p.Name, Kind: p.Kind}
					order = append(order, p.ID)
				}
				perAmt[p.ID] = perAmt[p.ID].Add(p.Amount)
			}
			all = all.Add(r.Total)
			res.Data = append(res.Data, ListRow{ID: r.ID, DocNo: r.DocNo, Status: r.Status, CreatedAt: r.CreatedAt.Time, Cashier: r.CashierName,
				Member: r.MemberName, LineCount: int(r.LineCount), Total: r.Total.StringFixed(2), Methods: methods, Pays: pays})
		}
		res.Total = all.StringFixed(2)
		for _, id := range order {
			m := perMethod[id]
			m.Amount = perAmt[id].StringFixed(2)
			res.ByMethod = append(res.ByMethod, *m)
		}
		sort.SliceStable(res.ByMethod, func(i, j int) bool {
			ci, cj := res.ByMethod[i].Kind == "cash", res.ByMethod[j].Kind == "cash"
			if ci != cj {
				return ci
			}
			return strings.ToLower(res.ByMethod[i].Name) < strings.ToLower(res.ByMethod[j].Name)
		})
		for m, v := range sums {
			res.Totals[m] = v.StringFixed(2)
		}
		return nil
	})
	return res, err
}

// List: GET /sales/?from=YYYY-MM-DD&to=YYYY-MM-DD&q=<no. nota>.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	res, err := h.svc.List(r.Context(), actor(r), qs.Get("from"), qs.Get("to"), qs.Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
