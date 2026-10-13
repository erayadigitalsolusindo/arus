package accounting

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

type PettyCashCategoryRow struct {
	Category string `json:"category"`
	Entries  int64  `json:"entries"`
	In       string `json:"in"`
	Out      string `json:"out"`
	Net      string `json:"net"`
}

type PettyCashReport struct {
	From  string                 `json:"from"`
	To    string                 `json:"to"`
	Rows  []PettyCashCategoryRow `json:"rows"`
	Total PettyCashCategoryRow   `json:"total"`
}

// PettyCashReport groups valid drawer movements by category using each outlet's business timezone.
func (s *Service) PettyCashReport(ctx context.Context, a authz.Actor, outlet uuid.UUID, fromS, toS string) (PettyCashReport, error) {
	from, to, err := parseRange(fromS, toS)
	if err != nil {
		return PettyCashReport{}, err
	}
	outlets, err := scopeOutlets(a, outlet)
	if err != nil {
		return PettyCashReport{}, err
	}
	out := PettyCashReport{From: from.Format(dateLayout), To: to.Format(dateLayout), Rows: []PettyCashCategoryRow{}}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH scoped_outlets AS (
				SELECT id, timezone,
					(($3::date)::timestamp AT TIME ZONE timezone) AS starts_at,
					((($4::date + 1)::timestamp) AT TIME ZONE timezone) AS ends_at
				FROM outlets
				WHERE tenant_id = $1 AND ($2::uuid[] IS NULL OR id = ANY($2))
			)
			SELECT m.category, count(*),
				coalesce(sum(m.amount) FILTER (WHERE m.direction = 'in'), 0),
				coalesce(sum(m.amount) FILTER (WHERE m.direction = 'out'), 0)
			FROM scoped_outlets o
			JOIN cash_movements m ON m.tenant_id = $1 AND m.outlet_id = o.id
				AND m.created_at >= o.starts_at AND m.created_at < o.ends_at
				AND m.voided_at IS NULL
			GROUP BY m.category ORDER BY m.category`, a.TenantID, outlets, pgDate(from), pgDate(to))
		if err != nil {
			return err
		}
		defer rows.Close()
		totalIn, totalOut := decimal.Zero, decimal.Zero
		for rows.Next() {
			var row PettyCashCategoryRow
			var in, outAmount decimal.Decimal
			if err := rows.Scan(&row.Category, &row.Entries, &in, &outAmount); err != nil {
				return err
			}
			row.In, row.Out = str(in), str(outAmount)
			row.Net = str(in.Sub(outAmount))
			out.Rows = append(out.Rows, row)
			totalIn = totalIn.Add(in)
			totalOut = totalOut.Add(outAmount)
			out.Total.Entries += row.Entries
		}
		if err := rows.Err(); err != nil {
			return err
		}
		out.Total.In, out.Total.Out = str(totalIn), str(totalOut)
		out.Total.Net = str(totalIn.Sub(totalOut))
		return nil
	})
	return out, err
}
