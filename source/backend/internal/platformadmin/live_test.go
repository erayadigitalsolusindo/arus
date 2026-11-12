package platformadmin

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Fungsi live hanya untuk Platform Admin aktif; umpan terurut terbaru dulu dan cursor tidak tumpang tindih.
func TestLiveFeedAndSummary(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()

	if _, err := e.svc.LiveFeed(ctx, Actor{ID: uuid.New()}, nil, nil); err == nil {
		t.Fatal("bukan admin harus ditolak")
	}
	if _, err := e.svc.LiveSummary(ctx, Actor{ID: uuid.New()}, nil); err == nil {
		t.Fatal("bukan admin harus ditolak")
	}

	sum, err := e.svc.LiveSummary(ctx, a, nil)
	if err != nil || sum.SalesTotal == "" || sum.Online == nil {
		t.Fatalf("summary=%+v err=%v", sum, err)
	}
	page1, err := e.svc.LiveFeed(ctx, a, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(page1.Items)) > sum.SalesCount+10 { // nota baru boleh masuk di antara dua panggilan
		t.Errorf("umpan (%d) melebihi jumlah nota hari ini (%d)", len(page1.Items), sum.SalesCount)
	}
	for i := 1; i < len(page1.Items); i++ {
		if page1.Items[i].CreatedAt.After(page1.Items[i-1].CreatedAt) {
			t.Fatal("umpan harus terbaru dulu")
		}
	}
	if page1.NextAt != nil {
		page2, err := e.svc.LiveFeed(ctx, a, page1.NextAt, page1.NextID)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[uuid.UUID]bool{}
		for _, x := range page1.Items {
			seen[x.ID] = true
		}
		for _, x := range page2.Items {
			if seen[x.ID] {
				t.Fatal("cursor tumpang tindih")
			}
		}
	}
}
