// Command apk-uploader: unggah APK ARUS Kasir dari PC Windows langsung ke server lewat SFTP (SSH),
// tanpa melewati Cloudflare/Apache/API. Berkas ditaruh di folder unggahan API beserta metadatanya,
// sehingga halaman login langsung menampilkan tombol unduh.
package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	apkName  = "arus-kasir.apk"
	infoName = "arus-kasir-info.json"
	maxBytes = 150 << 20 // sama dengan MaxAPKBytes di API
)

var versionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$`)

func main() {
	host := flag.String("host", "200.200.200.2", "alamat server")
	port := flag.Int("port", 22, "port SSH")
	user := flag.String("user", "root", "pengguna SSH")
	dir := flag.String("dir", "/www/wwwroot/arus/data/uploads/mobile", "folder tujuan di server (UPLOAD_DIR + /mobile)")
	owner := flag.String("owner", "www:www", "pemilik berkas di server (pengguna service API); kosong = jangan diubah")
	version := flag.String("version", "", "versi aplikasi, mis. 1.0.3 (kosong = ditanya)")
	flag.Parse()

	in := bufio.NewReader(os.Stdin)
	err := run(in, *host, *port, *user, *dir, *owner, *version, flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nGAGAL:", err)
	}
	fmt.Print("\nTekan Enter untuk menutup...")
	in.ReadString('\n')
	if err != nil {
		os.Exit(1)
	}
}

func run(in *bufio.Reader, host string, port int, user, dir, owner, version, file string) error {
	if file == "" {
		file = ask(in, "Path berkas APK (atau seret berkas ke jendela ini): ")
	}
	file = strings.Trim(strings.TrimSpace(file), `"`)
	size, err := checkAPK(file)
	if err != nil {
		return err
	}
	if version == "" {
		version = ask(in, "Versi (mis. 1.0.3, kosongkan bila tak perlu): ")
	}
	if version != "" && !versionRe.MatchString(version) {
		return errors.New("versi hanya huruf, angka, titik, minus, plus (maks 32 karakter)")
	}
	fmt.Printf("APK   : %s (%.1f MB)\nServer: %s@%s:%d\nTujuan: %s/%s\n\n", file, float64(size)/1e6, user, host, port, dir, apkName)

	fmt.Printf("Password SSH untuk %s: ", user)
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("baca password: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(string(pw))},
		HostKeyCallback: hostKeyCheck(in),
		Timeout:         15 * time.Second,
	}
	conn, err := ssh.Dial("tcp", net.JoinHostPort(host, fmt.Sprint(port)), cfg)
	if err != nil {
		return fmt.Errorf("tidak bisa masuk ke server: %w", err)
	}
	defer conn.Close()
	cl, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("SFTP: %w", err)
	}
	defer cl.Close()
	if err := cl.MkdirAll(dir); err != nil {
		return fmt.Errorf("buat folder %s: %w", dir, err)
	}

	sum, err := upload(cl, file, path.Join(dir, apkName), size)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{
		"size": size, "version": version, "sha256": sum, "uploaded_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err := putAtomic(cl, path.Join(dir, infoName), raw); err != nil {
		return fmt.Errorf("tulis metadata: %w", err)
	}

	if owner != "" {
		sess, err := conn.NewSession()
		if err == nil {
			out, cerr := sess.CombinedOutput(fmt.Sprintf("chown %s '%s/%s' '%s/%s' && chmod 640 '%s/%s' '%s/%s'", owner, dir, apkName, dir, infoName, dir, apkName, dir, infoName))
			sess.Close()
			if cerr != nil {
				fmt.Printf("PERINGATAN: gagal mengatur pemilik berkas (%v) %s\nJalankan di server: chown -R %s %s\n", cerr, strings.TrimSpace(string(out)), owner, dir)
			}
		}
	}
	fmt.Printf("\nSELESAI. SHA-256: %s\nCek: https://arus-api.erayadigital.co.id/public/mobile/apk/info (available harus true)\n", sum)
	return nil
}

// checkAPK memastikan berkas ada, ≤150 MB, dan berupa ZIP berisi AndroidManifest.xml.
func checkAPK(p string) (int64, error) {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return 0, fmt.Errorf("berkas tidak ditemukan: %s", p)
	}
	if st.Size() > maxBytes {
		return 0, fmt.Errorf("APK %.1f MB melebihi batas 150 MB (bangun dengan --split-per-abi)", float64(st.Size())/1e6)
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return 0, errors.New("bukan berkas APK yang sah (bukan ZIP)")
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "AndroidManifest.xml" {
			return st.Size(), nil
		}
	}
	return 0, errors.New("bukan berkas APK yang sah (tanpa AndroidManifest.xml)")
}

// upload mengirim ke berkas sementara lalu me-rename, agar unduhan tak pernah melihat APK setengah jadi.
func upload(cl *sftp.Client, local, remote string, size int64) (string, error) {
	src, err := os.Open(local)
	if err != nil {
		return "", err
	}
	defer src.Close()
	tmp := remote + ".part"
	dst, err := cl.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("buat %s: %w", tmp, err)
	}
	h := sha256.New()
	pr := &progress{r: io.TeeReader(src, h), total: size, start: time.Now()}
	_, err = io.Copy(dst, pr)
	cerr := dst.Close()
	fmt.Println()
	if err != nil || cerr != nil {
		cl.Remove(tmp)
		return "", fmt.Errorf("kirim gagal: %v %v", err, cerr)
	}
	if st, err := cl.Stat(tmp); err != nil || st.Size() != size {
		cl.Remove(tmp)
		return "", errors.New("ukuran berkas di server tidak sama; unggahan dibatalkan")
	}
	if err := replace(cl, tmp, remote); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func putAtomic(cl *sftp.Client, remote string, data []byte) error {
	tmp := remote + ".part"
	f, err := cl.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return replace(cl, tmp, remote)
}

func replace(cl *sftp.Client, tmp, remote string) error {
	if err := cl.PosixRename(tmp, remote); err == nil {
		return nil
	}
	cl.Remove(remote)
	if err := cl.Rename(tmp, remote); err != nil {
		return fmt.Errorf("ganti %s: %w", remote, err)
	}
	return nil
}

type progress struct {
	r     io.Reader
	total int64
	done  int64
	start time.Time
	last  time.Time
}

func (p *progress) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if time.Since(p.last) > 300*time.Millisecond || p.done == p.total {
		p.last = time.Now()
		sec := time.Since(p.start).Seconds()
		fmt.Printf("\rMengirim... %3d%%  %.1f MB/s ", p.done*100/p.total, float64(p.done)/1e6/max(sec, 0.001))
	}
	return n, err
}

// hostKeyCheck: trust-on-first-use. Sidik jari disimpan di known_host.txt di samping exe; bila berubah, batalkan.
func hostKeyCheck(in *bufio.Reader) ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		fp := ssh.FingerprintSHA256(key)
		exe, _ := os.Executable()
		store := filepath.Join(filepath.Dir(exe), "known_host.txt")
		if b, err := os.ReadFile(store); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				f := strings.Fields(line)
				if len(f) == 2 && f[0] == hostname {
					if f[1] == fp {
						return nil
					}
					return fmt.Errorf("SIDIK JARI SERVER BERUBAH (%s). Bisa jadi server diganti atau ada penyadapan. Hapus %s bila Anda yakin", fp, store)
				}
			}
		}
		fmt.Printf("Server %s belum dikenal.\nSidik jari: %s\n", hostname, fp)
		if !strings.EqualFold(ask(in, "Percayai server ini? (y/N): "), "y") {
			return errors.New("server tidak dipercaya")
		}
		f, err := os.OpenFile(store, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%s %s\n", hostname, fp)
			f.Close()
		}
		return nil
	}
}

func ask(in *bufio.Reader, q string) string {
	fmt.Print(q)
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}
