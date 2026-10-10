// Command print-agent: penghubung lokal antara aplikasi ARUS (browser) dan printer thermal di PC kasir.
// Aplikasi menyusun struk sebagai perintah ESC/POS; agent hanya meneruskannya apa adanya ke printer (RAW),
// sehingga perubahan tampilan struk cukup dengan memperbarui aplikasi web, tanpa memasang ulang agent.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	dir := exeDir()
	cfgPath := flag.String("config", filepath.Join(dir, "print-agent.json"), "berkas konfigurasi")
	test := flag.Bool("test", false, "cetak halaman uji ke printer lalu keluar")
	flag.Parse()

	logger := openLog(filepath.Join(dir, "print-agent.log"))
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		logger.Fatalf("konfigurasi: %v", err)
	}
	p, err := newPrinter(cfg)
	if err != nil {
		logger.Fatalf("printer: %v", err)
	}
	if *test {
		if err := p.Print("ARUS uji", testPage()); err != nil {
			logger.Fatalf("uji cetak ke %s gagal: %v", p.Name(), err)
		}
		logger.Printf("uji cetak terkirim ke %s", p.Name())
		return
	}

	s := &server{cfg: cfg, printer: p, log: logger}
	srv := &http.Server{Addr: cfg.Listen, Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	logger.Printf("print-agent %s mendengar di http://%s, printer: %s, origins: %v", version, cfg.Listen, p.Name(), cfg.Origins)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// Paling sering: agent lain sudah berjalan di port yang sama.
		logger.Fatalf("server: %v", err)
	}
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// openLog menulis ke berkas di samping program (versi Windows berjalan tanpa jendela konsol) dan stderr.
// Berkas di atas 1 MB dikosongkan saat agent mulai agar tidak tumbuh tanpa batas.
func openLog(path string) *log.Logger {
	if st, err := os.Stat(path); err == nil && st.Size() > 1<<20 {
		_ = os.Truncate(path, 0)
	}
	var w io.Writer = os.Stderr
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		w = io.MultiWriter(f, os.Stderr)
	}
	return log.New(w, "", log.LstdFlags)
}

// testPage = halaman uji ESC/POS sederhana untuk memastikan jalur ke printer bekerja (tanpa aplikasi).
func testPage() []byte {
	const esc, gs = "\x1b", "\x1d"
	s := esc + "@" + esc + "a\x01" + gs + "!\x11" + "ARUS\n" + gs + "!\x00" + "print-agent " + version + "\n" +
		esc + "a\x00" + "--------------------------------\n" +
		"12345678901234567890123456789012\n" + esc + "E\x01" + "Tebal" + esc + "E\x00" + " / normal\n" +
		gs + "!\x01" + "Tinggi ganda\n" + gs + "!\x00" +
		fmt.Sprintf("%s\n", time.Now().Format("02/01/2006 15:04:05")) + esc + "d\x04" + gs + "V\x42\x00"
	return []byte(s)
}
