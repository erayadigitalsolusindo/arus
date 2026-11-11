package backup

// Backup terjadwal ke Google Drive (migrasi 00108). pg_dump dijalankan dengan role PEMILIK skema (MIGRATE_DATABASE_URL): role aplikasi
// terkena RLS sehingga dumpnya akan kosong/terpotong diam-diam, karena itu dicek di tiap proses. Dump dialirkan
// pg_dump → enkripsi (passphrase operator) → Drive tanpa berkas sementara. Restore sungguhan sengaja TIDAK ada di panel; yang ada
// hanya uji restore ke database sementara.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	lockKey     = "backup:lock"
	lockTTL     = 4 * time.Hour
	runTimeout  = 3 * time.Hour
	tickEvery   = time.Minute
	maxAutoTry  = 3
	oauthTTL    = 10 * time.Minute
	redirectSuf = "/platform/backup/callback"
)

var (
	ErrNotReady         = errors.New("backup belum siap (Drive belum tersambung atau passphrase belum dibuat)")
	ErrBusy             = errors.New("backup lain sedang berjalan")
	ErrBadInput         = errors.New("isian tidak valid")
	ErrBadState         = errors.New("state OAuth tidak valid atau kedaluwarsa")
	ErrPassphraseExists = errors.New("passphrase sudah dibuat")
	ErrNoOwnerURL       = errors.New("MIGRATE_DATABASE_URL belum diatur di server")
	ErrNotOwner         = errors.New("MIGRATE_DATABASE_URL bukan role pemilik skema (dump akan terpotong oleh RLS)")
	ErrNoBackup         = errors.New("belum ada backup yang berhasil untuk diuji")

	wib = time.FixedZone("WIB", 7*3600)
)

type Service struct {
	pool     *pgxpool.Pool
	rdb      *redis.Client
	box      *sealer
	ownerURL string
	upload   string // folder unggahan lokal (ikut dicadangkan)
	redirect string
	log      *slog.Logger
}

func NewService(pool *pgxpool.Pool, rdb *redis.Client, jwtSecret, ownerURL, uploadDir, appBaseURL string, log *slog.Logger) (*Service, error) {
	box, err := newSealer(jwtSecret)
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, rdb: rdb, box: box, ownerURL: ownerURL, upload: uploadDir,
		redirect: strings.TrimRight(appBaseURL, "/") + redirectSuf, log: log}, nil
}

// Config = status konfigurasi untuk panel (tanpa rahasia).
type Config struct {
	ClientID       string    `json:"client_id"`
	HasSecret      bool      `json:"has_secret"`
	Connected      bool      `json:"connected"`
	Account        string    `json:"account"`
	HasPassphrase  bool      `json:"has_passphrase"`
	Enabled        bool      `json:"enabled"`
	RunHour        int       `json:"run_hour"`
	KeepCount      int       `json:"keep_count"`
	IncludeUploads bool      `json:"include_uploads"`
	UpdatedAt      time.Time `json:"updated_at"`
	RedirectURI    string    `json:"redirect_uri"`
	PgDump         bool      `json:"pg_dump"`
	PgRestore      bool      `json:"pg_restore"`
	OwnerURL       bool      `json:"owner_url"`
}

func (s *Service) Get(ctx context.Context, admin uuid.UUID) (Config, error) {
	var c Config
	var folder string
	err := s.pool.QueryRow(ctx, `SELECT client_id, has_secret, connected, account_email, folder_id, has_passphrase, enabled, run_hour,
		keep_count, include_uploads, updated_at FROM platform_backup_get($1)`, admin).
		Scan(&c.ClientID, &c.HasSecret, &c.Connected, &c.Account, &folder, &c.HasPassphrase, &c.Enabled, &c.RunHour, &c.KeepCount,
			&c.IncludeUploads, &c.UpdatedAt)
	c.RedirectURI = s.redirect
	_, e1 := exec.LookPath("pg_dump")
	_, e2 := exec.LookPath("pg_restore")
	c.PgDump, c.PgRestore, c.OwnerURL = e1 == nil, e2 == nil, s.ownerURL != ""
	return c, err
}

type SaveInput struct {
	ClientID, ClientSecret  string // secret kosong = pertahankan
	Enabled, IncludeUploads bool
	RunHour, KeepCount      int
}

func (s *Service) Save(ctx context.Context, admin uuid.UUID, in SaveInput) error {
	in.ClientID = strings.TrimSpace(in.ClientID)
	in.ClientSecret = strings.TrimSpace(in.ClientSecret)
	if len(in.ClientID) > 300 || len(in.ClientSecret) > 300 || in.RunHour < 0 || in.RunHour > 23 || in.KeepCount < 1 || in.KeepCount > 365 {
		return ErrBadInput
	}
	enc := ""
	if in.ClientSecret != "" {
		var err error
		if enc, err = s.box.seal(in.ClientSecret, "client_secret"); err != nil {
			return err
		}
	}
	_, err := s.pool.Exec(ctx, `SELECT platform_backup_save($1, $2, $3, $4, $5, $6, $7)`,
		admin, in.ClientID, enc, in.Enabled, in.RunHour, in.KeepCount, in.IncludeUploads)
	return err
}

type loaded struct {
	clientID, secret, refresh, folder, pass string
	enabled, uploads                        bool
	hour, keep                              int
}

func (s *Service) load(ctx context.Context) (loaded, error) {
	var l loaded
	var secEnc, refEnc, passEnc string
	if err := s.pool.QueryRow(ctx, `SELECT client_id, secret_enc, refresh_enc, folder_id, passphrase_enc, enabled, run_hour, keep_count,
		include_uploads FROM backup_config_load()`).Scan(&l.clientID, &secEnc, &refEnc, &l.folder, &passEnc, &l.enabled, &l.hour, &l.keep, &l.uploads); err != nil {
		return l, err
	}
	var err error
	if secEnc != "" {
		if l.secret, err = s.box.open(secEnc, "client_secret"); err != nil {
			return l, fmt.Errorf("backup: rahasia tak bisa dibuka (JWT_SECRET berubah?): %w", err)
		}
	}
	if refEnc != "" {
		if l.refresh, err = s.box.open(refEnc, "refresh_token"); err != nil {
			return l, fmt.Errorf("backup: token tak bisa dibuka (JWT_SECRET berubah?): %w", err)
		}
	}
	if passEnc != "" {
		if l.pass, err = s.box.open(passEnc, "passphrase"); err != nil {
			return l, fmt.Errorf("backup: passphrase tak bisa dibuka (JWT_SECRET berubah?): %w", err)
		}
	}
	return l, nil
}

func (l loaded) ready() bool {
	return l.clientID != "" && l.secret != "" && l.refresh != "" && l.pass != ""
}

// AuthURL = tautan persetujuan Google; state sekali pakai terikat pada admin yang memintanya.
func (s *Service) AuthURL(ctx context.Context, admin uuid.UUID) (string, error) {
	l, err := s.load(ctx)
	if err != nil {
		return "", err
	}
	if l.clientID == "" || l.secret == "" {
		return "", ErrBadInput
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)
	if err := s.rdb.Set(ctx, "backup:oauth:"+state, admin.String(), oauthTTL).Err(); err != nil {
		return "", err
	}
	return authLink(l.clientID, s.redirect, state), nil
}

// Connect menyelesaikan OAuth: menukar kode, membuat folder "ARUS Backup", menyimpan refresh token terenkripsi.
func (s *Service) Connect(ctx context.Context, admin uuid.UUID, code, state string) (account string, err error) {
	got, gerr := s.rdb.GetDel(ctx, "backup:oauth:"+state).Result()
	if gerr != nil || got != admin.String() || code == "" || len(code) > 2000 {
		return "", ErrBadState
	}
	l, err := s.load(ctx)
	if err != nil {
		return "", err
	}
	d := newDrive(l.clientID, l.secret, "")
	refresh, email, err := d.exchange(ctx, code, s.redirect)
	if err != nil {
		return "", err
	}
	d.refreshTok = refresh
	folder, _, err := d.ensureFolder(ctx, "")
	if err != nil {
		return "", err
	}
	enc, err := s.box.seal(refresh, "refresh_token")
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `SELECT platform_backup_connect($1, $2, $3, $4)`, admin, enc, email, folder)
	return email, err
}

func (s *Service) Disconnect(ctx context.Context, admin uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `SELECT platform_backup_disconnect($1)`, admin)
	return err
}

// CreatePassphrase membuat passphrase enkripsi satu kali dan mengembalikannya (hanya saat ini ia terlihat).
func (s *Service) CreatePassphrase(ctx context.Context, admin uuid.UUID) (string, error) {
	p, err := NewPassphrase()
	if err != nil {
		return "", err
	}
	enc, err := s.box.seal(p, "passphrase")
	if err != nil {
		return "", err
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT platform_backup_set_passphrase($1, $2)`, admin, enc).Scan(&ok); err != nil {
		return "", err
	}
	if !ok {
		return "", ErrPassphraseExists
	}
	return p, nil
}

type Run struct {
	ID           uuid.UUID  `json:"id"`
	Kind         string     `json:"kind"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	DBName       string     `json:"db_name"`
	DBBytes      int64      `json:"db_bytes"`
	UploadsBytes int64      `json:"uploads_bytes"`
	Deleted      bool       `json:"deleted"`
	Detail       string     `json:"detail"`
}

func (s *Service) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, kind, status, started_at, finished_at, db_name, db_bytes, uploads_bytes,
		deleted_at IS NOT NULL, detail FROM backup_runs ORDER BY started_at DESC, id DESC LIMIT $1`, min(max(limit, 1), 100))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.Kind, &r.Status, &r.StartedAt, &r.FinishedAt, &r.DBName, &r.DBBytes, &r.UploadsBytes, &r.Deleted, &r.Detail); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Start menjalankan backup manual di latar belakang dan langsung mengembalikan id proses.
func (s *Service) Start(ctx context.Context, kind string) (uuid.UUID, error) {
	l, err := s.load(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	if !l.ready() {
		return uuid.Nil, ErrNotReady
	}
	return s.launch(ctx, kind, func(rc context.Context, id uuid.UUID) (string, error) { return s.backup(rc, id, l) })
}

// StartRestoreTest memulihkan backup terbaru ke database sementara lalu menghapusnya lagi.
func (s *Service) StartRestoreTest(ctx context.Context) (uuid.UUID, error) {
	l, err := s.load(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	if !l.ready() {
		return uuid.Nil, ErrNotReady
	}
	return s.launch(ctx, "restore_test", func(rc context.Context, id uuid.UUID) (string, error) { return s.restoreTest(rc, l) })
}

func (s *Service) launch(ctx context.Context, kind string, work func(context.Context, uuid.UUID) (string, error)) (uuid.UUID, error) {
	id := uuid.New()
	ok, err := s.rdb.SetNX(ctx, lockKey, id.String(), lockTTL).Result()
	if err != nil {
		return uuid.Nil, err
	}
	if !ok {
		return uuid.Nil, ErrBusy
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO backup_runs (id, kind) VALUES ($1, $2)`, id, kind); err != nil {
		s.rdb.Del(ctx, lockKey)
		return uuid.Nil, err
	}
	go func() {
		rc, cancel := context.WithTimeout(context.Background(), runTimeout)
		defer cancel()
		defer s.rdb.Del(context.Background(), lockKey)
		detail, err := work(rc, id)
		status := "ok"
		if err != nil {
			status, detail = "failed", truncate(err.Error(), 900)
			s.log.Error("backup gagal", "kind", kind, "err", err)
		}
		if _, e := s.pool.Exec(context.Background(), `UPDATE backup_runs SET status = $2, finished_at = now(), detail = $3 WHERE id = $1`, id, status, detail); e != nil {
			s.log.Error("backup: gagal mencatat hasil", "err", e)
		}
	}()
	return id, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Scheduler berjalan sampai ctx selesai: tiap menit memulai backup harian bila sudah lewat jam jadwal (WIB) dan belum ada yang sukses
// hari itu. Gagal diulang paling banyak 3 kali per hari dengan jeda 30 menit. Redis NX mencegah ganda antar instance API.
func (s *Service) Scheduler(ctx context.Context) {
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Service) tick(ctx context.Context) {
	// proses yang terputus (API restart) ditandai gagal agar tidak menggantung selamanya
	_, _ = s.pool.Exec(ctx, `UPDATE backup_runs SET status = 'failed', finished_at = now(), detail = 'terputus (server dimulai ulang?)'
		WHERE status = 'running' AND started_at < now() - interval '4 hours'`)
	l, err := s.load(ctx)
	if err != nil || !l.enabled || !l.ready() {
		return
	}
	now := time.Now().In(wib)
	boundary := time.Date(now.Year(), now.Month(), now.Day(), l.hour, 0, 0, 0, wib)
	if now.Before(boundary) {
		return
	}
	var tries, oks int
	var last *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status = 'ok'), max(started_at) FROM backup_runs
		WHERE kind = 'auto' AND started_at >= $1`, boundary).Scan(&tries, &oks, &last); err != nil {
		return
	}
	if oks > 0 || tries >= maxAutoTry || (last != nil && time.Since(*last) < 30*time.Minute) {
		return
	}
	if _, err := s.launch(ctx, "auto", func(rc context.Context, id uuid.UUID) (string, error) { return s.backup(rc, id, l) }); err != nil && !errors.Is(err, ErrBusy) {
		s.log.Error("backup terjadwal gagal dimulai", "err", err)
	}
}

type dbConn struct{ host, port, user, pass, name, ssl string }

func parseConn(raw string) (dbConn, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return dbConn{}, ErrNoOwnerURL
	}
	c := dbConn{host: u.Hostname(), port: u.Port(), user: u.User.Username(), name: strings.TrimPrefix(u.Path, "/"), ssl: u.Query().Get("sslmode")}
	c.pass, _ = u.User.Password()
	if c.port == "" {
		c.port = "5432"
	}
	return c, nil
}

func (c dbConn) args(db string) []string {
	return []string{"-h", c.host, "-p", c.port, "-U", c.user, "-d", db}
}

func (c dbConn) env() []string {
	e := append(os.Environ(), "PGPASSWORD="+c.pass)
	if c.ssl != "" {
		e = append(e, "PGSSLMODE="+c.ssl)
	}
	return e
}

// checkOwner menolak role yang akan terkena RLS (dump kosong tanpa galat).
func (s *Service) checkOwner(ctx context.Context) (dbConn, error) {
	if s.ownerURL == "" {
		return dbConn{}, ErrNoOwnerURL
	}
	c, err := parseConn(s.ownerURL)
	if err != nil {
		return c, err
	}
	conn, err := pgx.Connect(ctx, s.ownerURL)
	if err != nil {
		return c, fmt.Errorf("backup: koneksi pemilik gagal: %w", err)
	}
	defer conn.Close(ctx)
	var ok bool
	err = conn.QueryRow(ctx, `SELECT pg_get_userbyid(c.relowner) = current_user
		OR EXISTS (SELECT 1 FROM pg_roles WHERE rolname = current_user AND (rolsuper OR rolbypassrls))
		FROM pg_class c WHERE c.relname = 'tenants' AND c.relnamespace = 'public'::regnamespace`).Scan(&ok)
	if err != nil || !ok {
		return c, ErrNotOwner
	}
	return c, nil
}

type tail struct{ b []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 1500 {
		t.b = t.b[len(t.b)-1500:]
	}
	return len(p), nil
}

// streamUpload: produce menulis plaintext → dienkripsi → diunggah. Galat produce membatalkan unggahan (berkas tidak terbentuk).
func streamUpload(ctx context.Context, d *drive, folder, name, pass string, produce func(io.Writer) error) (string, int64, error) {
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ew, err := Encrypt(pw, pass)
		if err == nil {
			if err = produce(ew); err == nil {
				err = ew.Close()
			}
		}
		pw.CloseWithError(err)
	}()
	id, n, err := d.upload(ctx, folder, name, pr)
	pr.CloseWithError(errors.New("unggah selesai"))
	<-done
	return id, n, err
}

func (s *Service) backup(ctx context.Context, runID uuid.UUID, l loaded) (string, error) {
	c, err := s.checkOwner(ctx)
	if err != nil {
		return "", err
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return "", errors.New("pg_dump tidak ditemukan di server (pasang postgresql-client yang versinya ≥ server)")
	}
	d := newDrive(l.clientID, l.secret, l.refresh)
	folder, changed, err := d.ensureFolder(ctx, l.folder)
	if err != nil {
		return "", err
	}
	if changed {
		_, _ = s.pool.Exec(ctx, `SELECT backup_set_folder($1)`, folder)
	}
	stamp := time.Now().In(wib).Format("20060102-150405")
	dbName := "arus-db-" + stamp + ".dump.enc"

	cmd := exec.CommandContext(ctx, "pg_dump", append([]string{"-Fc"}, c.args(c.name)...)...)
	cmd.Env = c.env()
	var stderr tail
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	dbID, dbN, err := streamUpload(ctx, d, folder, dbName, l.pass, func(w io.Writer) error {
		_, cerr := io.Copy(w, out)
		if cerr != nil {
			_ = cmd.Process.Kill()
		}
		werr := cmd.Wait()
		if cerr != nil {
			return cerr
		}
		if werr != nil {
			return fmt.Errorf("pg_dump gagal: %w: %s", werr, strings.TrimSpace(string(stderr.b)))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE backup_runs SET db_name = $2, db_file_id = $3, db_bytes = $4 WHERE id = $1`, runID, dbName, dbID, dbN)

	detail := ""
	if l.uploads {
		if _, serr := os.Stat(s.upload); serr == nil {
			upName := "arus-uploads-" + stamp + ".tar.gz.enc"
			upID, upN, uerr := streamUpload(ctx, d, folder, upName, l.pass, func(w io.Writer) error { return tarGz(w, s.upload) })
			if uerr != nil {
				_ = d.remove(context.Background(), dbID) // jangan tinggalkan separuh pasangan
				_, _ = s.pool.Exec(ctx, `UPDATE backup_runs SET db_file_id = '' WHERE id = $1`, runID)
				return "", fmt.Errorf("unggahan gambar gagal: %w", uerr)
			}
			_, _ = s.pool.Exec(ctx, `UPDATE backup_runs SET uploads_name = $2, uploads_file_id = $3, uploads_bytes = $4 WHERE id = $1`, runID, upName, upID, upN)
		}
	}
	if n := s.prune(ctx, d, l.keep); n > 0 {
		detail = fmt.Sprintf("%d salinan lama dihapus (simpan %d)", n, l.keep)
	}
	return detail, nil
}

// prune menghapus salinan berhasil terlama di luar `keep` terbaru (berkas Drive + tanda deleted_at).
func (s *Service) prune(ctx context.Context, d *drive, keep int) int {
	rows, err := s.pool.Query(ctx, `SELECT id, db_file_id, uploads_file_id FROM backup_runs
		WHERE status = 'ok' AND kind IN ('auto', 'manual') AND deleted_at IS NULL
		ORDER BY started_at DESC, id DESC OFFSET $1 LIMIT 100`, keep)
	if err != nil {
		return 0
	}
	type old struct {
		id     uuid.UUID
		d1, d2 string
	}
	var list []old
	for rows.Next() {
		var o old
		if rows.Scan(&o.id, &o.d1, &o.d2) == nil {
			list = append(list, o)
		}
	}
	rows.Close()
	n := 0
	for _, o := range list {
		okAll := true
		for _, f := range []string{o.d1, o.d2} {
			if f != "" && d.remove(ctx, f) != nil {
				okAll = false
			}
		}
		if okAll {
			_, _ = s.pool.Exec(ctx, `UPDATE backup_runs SET deleted_at = now() WHERE id = $1`, o.id)
			n++
		}
	}
	return n
}

// tarGz mengarsipkan berkas biasa di bawah root (tautan simbolik dilewati agar tak keluar dari folder).
func tarGz(w io.Writer, root string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	base := filepath.Base(root)
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !de.Type().IsRegular() && !de.IsDir() {
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(filepath.Join(base, rel))
		if err := tw.WriteHeader(h); err != nil || de.IsDir() {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func (s *Service) restoreTest(ctx context.Context, l loaded) (string, error) {
	c, err := s.checkOwner(ctx)
	if err != nil {
		return "", err
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		return "", errors.New("pg_restore tidak ditemukan di server (pasang postgresql-client)")
	}
	var fileID, name string
	if err := s.pool.QueryRow(ctx, `SELECT db_file_id, db_name FROM backup_runs WHERE status = 'ok' AND kind IN ('auto', 'manual')
		AND deleted_at IS NULL AND db_file_id <> '' ORDER BY started_at DESC LIMIT 1`).Scan(&fileID, &name); err != nil {
		return "", ErrNoBackup
	}
	d := newDrive(l.clientID, l.secret, l.refresh)
	body, err := d.download(ctx, fileID)
	if err != nil {
		return "", err
	}
	defer body.Close()
	plain, err := Decrypt(body, l.pass)
	if err != nil {
		return "", err
	}

	tmp := "arus_rt_" + time.Now().UTC().Format("20060102150405")
	admin, err := pgx.Connect(ctx, s.ownerURL)
	if err != nil {
		return "", err
	}
	defer admin.Close(context.Background())
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{tmp}.Sanitize()); err != nil {
		return "", fmt.Errorf("tak bisa membuat database uji (role butuh CREATEDB): %w", err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+pgx.Identifier{tmp}.Sanitize()+` WITH (FORCE)`)
	}()

	cmd := exec.CommandContext(ctx, "pg_restore", append([]string{"--no-owner", "--no-privileges"}, c.args(tmp)...)...)
	cmd.Env, cmd.Stdin = c.env(), plain
	var stderr tail
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pg_restore gagal: %w: %s", err, strings.TrimSpace(string(stderr.b)))
	}
	u, _ := url.Parse(s.ownerURL)
	u.Path = "/" + tmp
	chk, err := pgx.Connect(ctx, u.String())
	if err != nil {
		return "", err
	}
	defer chk.Close(context.Background())
	var tenants, ver int64
	var hasSales bool
	// tanpa count(*) atas tabel transaksi: cukup bukti tabel ada dan terisi
	if err := chk.QueryRow(ctx, `SELECT (SELECT count(*) FROM tenants), EXISTS (SELECT 1 FROM sales),
		COALESCE((SELECT max(version_id) FROM goose_db_version), 0)`).Scan(&tenants, &hasSales, &ver); err != nil {
		return "", fmt.Errorf("verifikasi hasil restore gagal: %w", err)
	}
	return fmt.Sprintf("%s dipulihkan ke database sementara: tenant=%d, ada penjualan=%t, versi migrasi=%d", name, tenants, hasSales, ver), nil
}
