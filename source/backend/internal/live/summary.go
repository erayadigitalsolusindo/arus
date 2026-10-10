package live

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Hour struct {
	Hour  int    `json:"hour"`
	Count int    `json:"count"`
	Total string `json:"total"`
}

type Recent struct {
	ID        uuid.UUID `json:"id"`
	DocNo     string    `json:"doc_no"`
	CreatedAt time.Time `json:"created_at"`
	Cashier   string    `json:"cashier"`
	Member    string    `json:"member,omitempty"`
	LineCount int       `json:"line_count"`
	Total     string    `json:"total"`
}

type Totals struct {
	Count int    `json:"count"`
	Total string `json:"total"`
}

// Summary = penjualan HARI INI (zona waktu outlet aktif). Omzet = Σ total nota selesai (tanpa biaya metode);
// Returns menurut waktu dibuatnya retur; Net = omzet − retur. Yesterday = kemarin sampai jam yang sama (pembanding adil).
type Summary struct {
	Date      string    `json:"date"`
	Timezone  string    `json:"timezone"`
	AsOf      time.Time `json:"as_of"`
	Sales     Totals    `json:"sales"`
	Average   string    `json:"average"`
	Returns   Totals    `json:"returns"`
	Net       string    `json:"net"`
	Yesterday Totals    `json:"yesterday"`
	Hours     []Hour    `json:"hours"` // 24 elemen, jam lokal 0..23
	Recent    []Recent  `json:"recent"`
}

const boundsSQL = `
SELECT o.timezone, (now() AT TIME ZONE o.timezone)::date::text,
       (now() AT TIME ZONE o.timezone)::date::timestamp AT TIME ZONE o.timezone,
       ((now() AT TIME ZONE o.timezone)::date + 1)::timestamp AT TIME ZONE o.timezone,
       ((now() AT TIME ZONE o.timezone)::date - 1)::timestamp AT TIME ZONE o.timezone,
       now() - interval '1 day'
FROM outlets o WHERE o.tenant_id = $1 AND o.id = $2`

const hoursSQL = `
SELECT extract(hour FROM s.created_at AT TIME ZONE $4)::int, count(*), coalesce(sum(s.total), 0)
FROM sales s WHERE s.tenant_id = $1 AND s.outlet_id = $2 AND s.status = 'completed' AND s.created_at >= $3 AND s.created_at < $5
GROUP BY 1`

const recentSQL = `
SELECT s.id, s.doc_no, s.created_at, coalesce(u.name, '')::text, coalesce(mb.name, '')::text,
       (SELECT count(*) FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id)::int, s.total
FROM sales s
LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id
LEFT JOIN members mb ON mb.tenant_id = s.tenant_id AND mb.id = s.member_id
WHERE s.tenant_id = $1 AND s.outlet_id = $2 AND s.status = 'completed' AND s.created_at >= $3 AND s.created_at < $4
ORDER BY s.created_at DESC, s.doc_no DESC LIMIT 10`

func (s *Service) Today(ctx context.Context, a authz.Actor) (Summary, error) {
	out := Summary{Hours: make([]Hour, 24), Recent: []Recent{}}
	for i := range out.Hours {
		out.Hours[i] = Hour{Hour: i, Total: "0.00"}
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var t0, t1, y0, yNow time.Time
		if err := tx.QueryRow(ctx, boundsSQL, a.TenantID, a.OutletID).Scan(&out.Timezone, &out.Date, &t0, &t1, &y0, &yNow); err != nil {
			return err
		}
		out.AsOf = time.Now().UTC()
		total := decimal.Zero
		rows, err := tx.Query(ctx, hoursSQL, a.TenantID, a.OutletID, t0, out.Timezone, t1)
		if err != nil {
			return err
		}
		for rows.Next() {
			var h, n int
			var sum decimal.Decimal
			if err := rows.Scan(&h, &n, &sum); err != nil {
				rows.Close()
				return err
			}
			if h >= 0 && h < 24 {
				out.Hours[h] = Hour{Hour: h, Count: n, Total: sum.StringFixed(2)}
				out.Sales.Count += n
				total = total.Add(sum)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		out.Sales.Total = total.StringFixed(2)
		out.Average = "0.00"
		if out.Sales.Count > 0 {
			out.Average = total.Div(decimal.NewFromInt(int64(out.Sales.Count))).StringFixed(2)
		}

		var rc int
		var rt decimal.Decimal
		if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(sum(total), 0) FROM sales_returns
			WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND created_at >= $3 AND created_at < $4`,
			a.TenantID, a.OutletID, t0, t1).Scan(&rc, &rt); err != nil {
			return err
		}
		out.Returns = Totals{Count: rc, Total: rt.StringFixed(2)}
		out.Net = total.Sub(rt).StringFixed(2)

		var yc int
		var yt decimal.Decimal
		if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(sum(total), 0) FROM sales
			WHERE tenant_id = $1 AND outlet_id = $2 AND status = 'completed' AND created_at >= $3 AND created_at < $4`,
			a.TenantID, a.OutletID, y0, yNow).Scan(&yc, &yt); err != nil {
			return err
		}
		out.Yesterday = Totals{Count: yc, Total: yt.StringFixed(2)}

		rr, err := tx.Query(ctx, recentSQL, a.TenantID, a.OutletID, t0, t1)
		if err != nil {
			return err
		}
		defer rr.Close()
		for rr.Next() {
			var r Recent
			var tot decimal.Decimal
			if err := rr.Scan(&r.ID, &r.DocNo, &r.CreatedAt, &r.Cashier, &r.Member, &r.LineCount, &tot); err != nil {
				return err
			}
			r.Total = tot.StringFixed(2)
			out.Recent = append(out.Recent, r)
		}
		return rr.Err()
	})
	return out, err
}
