package sales

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/erapay"
)

// codeEraPayNotPaid: pembayaran QRIS EraPay di nota belum lunas / tidak cocok / sudah dipakai nota lain.
const codeEraPayNotPaid = "ERAPAY_NOT_PAID"

// PayVerifier memeriksa pembayaran yang bukti lunasnya ada di sistem lain (QRIS EraPay). Diimplementasikan erapay.Service.
type PayVerifier interface {
	// VerifyPayment: isEra=false bila methodID bukan metode yang dikelola verifier (abaikan).
	VerifyPayment(ctx context.Context, tx pgx.Tx, tenant, methodID uuid.UUID, ref string, amount decimal.Decimal, reuseSale *uuid.UUID) (chargeID uuid.UUID, isEra bool, err error)
	LinkPayment(ctx context.Context, tx pgx.Tx, tenant, chargeID, saleID uuid.UUID) error
}

// WithPayVerifier memasang pemeriksa pembayaran QRIS EraPay.
func (s *Service) WithPayVerifier(v PayVerifier) *Service {
	s.payVerifier = v
	return s
}

// verifyExternalPayments mengembalikan charge (indeks pembayaran -> id charge) yang harus ditautkan ke nota setelah tersimpan.
// Charge yang tak sah menjadi FieldErrors; galat lain dikembalikan apa adanya.
func (s *Service) verifyExternalPayments(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, n norm, reuseSale *uuid.UUID) (map[int]uuid.UUID, error) {
	if s.payVerifier == nil {
		return nil, nil
	}
	var out map[int]uuid.UUID
	for i, p := range n.payments {
		if p.methodID == nil {
			continue
		}
		id, isEra, err := s.payVerifier.VerifyPayment(ctx, tx, tenant, *p.methodID, p.refNo, p.amount, reuseSale)
		switch {
		case errors.Is(err, erapay.ErrChargeNotOK):
			return nil, FieldErrors{fmt.Sprintf("payments.%d.ref_no", i): codeEraPayNotPaid}
		case err != nil:
			return nil, err
		case !isEra:
			continue
		}
		if out == nil {
			out = map[int]uuid.UUID{}
		}
		out[i] = id
	}
	return out, nil
}

func (s *Service) linkExternalPayments(ctx context.Context, tx pgx.Tx, tenant, saleID uuid.UUID, charges map[int]uuid.UUID) error {
	for _, id := range charges {
		if err := s.payVerifier.LinkPayment(ctx, tx, tenant, id, saleID); err != nil {
			return err
		}
	}
	return nil
}
