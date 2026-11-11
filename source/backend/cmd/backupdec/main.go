// backupdec membuka berkas backup ARUS (*.enc) yang diunduh dari Google Drive. Passphrase dari env BACKUP_PASSPHRASE.
// Pakai:  BACKUP_PASSPHRASE=xxxx-... go run ./cmd/backupdec arus-db-....dump.enc > arus.dump   lalu  pg_restore -d <db> arus.dump
package main

import (
	"fmt"
	"io"
	"os"

	"aciraba/internal/backup"
)

func main() {
	pass := os.Getenv("BACKUP_PASSPHRASE")
	if len(os.Args) != 2 || pass == "" {
		fmt.Fprintln(os.Stderr, "pakai: BACKUP_PASSPHRASE=... backupdec <berkas.enc> > keluaran")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	r, err := backup.Decrypt(f, pass)
	if err == nil {
		_, err = io.Copy(os.Stdout, r)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal:", err)
		os.Exit(1)
	}
}
