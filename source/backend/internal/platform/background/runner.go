// Package background menjalankan pekerjaan lepas (kirim email, dll.) setelah respons HTTP dikirim, dengan jumlah
// bersamaan yang dibatasi dan timeout sendiri (tidak terikat context request yang sudah selesai).
// Untuk pekerjaan yang harus dijamin terkirim (retry/persisten) gunakan antrean (asynq) — bukan paket ini.
package background

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Runner struct {
	sem     chan struct{}
	timeout time.Duration
	log     *slog.Logger
	wg      sync.WaitGroup
}

func New(log *slog.Logger, maxConcurrent int, timeout time.Duration) *Runner {
	return &Runner{sem: make(chan struct{}, maxConcurrent), timeout: timeout, log: log}
}

// Go menjadwalkan fn. Bila sudah penuh, pekerjaan DIBUANG (dan dicatat) agar lonjakan tidak menumpuk goroutine;
// mengembalikan false pada kasus itu.
func (r *Runner) Go(name string, fn func(ctx context.Context) error) bool {
	select {
	case r.sem <- struct{}{}:
	default:
		r.log.Warn("pekerjaan latar belakang dibuang: antrean penuh", "job", name)
		return false
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() { <-r.sem }()
		defer func() {
			if rec := recover(); rec != nil {
				r.log.Error("pekerjaan latar belakang panik", "job", name, "recover", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			r.log.Error("pekerjaan latar belakang gagal", "job", name, "err", err)
		}
	}()
	return true
}

// Wait menunggu semua pekerjaan berjalan selesai (shutdown dan test).
func (r *Runner) Wait() { r.wg.Wait() }
