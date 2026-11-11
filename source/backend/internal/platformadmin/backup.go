package platformadmin

// Backup terjadwal ke Google Drive (internal/backup, migrasi 00108). Rahasia tidak pernah dikembalikan ke panel; passphrase enkripsi
// hanya tampil sekali saat dibuat. Semua aksi tercatat di audit platform.

import (
	"errors"
	"net/http"

	"aciraba/internal/backup"
	"aciraba/internal/platform/httpx"
)

const (
	ActionBackupSave       = "platform.backup_save"
	ActionBackupConnect    = "platform.backup_connect"
	ActionBackupDisconnect = "platform.backup_disconnect"
	ActionBackupPass       = "platform.backup_passphrase"
	ActionBackupRun        = "platform.backup_run"
	ActionBackupRestore    = "platform.backup_restore_test"
)

func (h *Handler) backupOff(w http.ResponseWriter) bool {
	if h.Backup == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "Fitur backup tidak tersedia.")
		return true
	}
	return false
}

// backupErr memetakan galat layanan; mengembalikan true bila sudah menulis respons.
func (h *Handler) backupErr(w http.ResponseWriter, r *http.Request, msg string, err error) {
	switch {
	case errors.Is(err, backup.ErrBadInput):
		httpx.Error(w, http.StatusUnprocessableEntity, "BACKUP_BAD_INPUT", "Isian tidak valid.")
	case errors.Is(err, backup.ErrBadState):
		httpx.Error(w, http.StatusUnprocessableEntity, "BACKUP_BAD_STATE", "Sesi persetujuan Google kedaluwarsa. Ulangi dari tombol Hubungkan.")
	case errors.Is(err, backup.ErrNotReady):
		httpx.Error(w, http.StatusConflict, "BACKUP_NOT_READY", "Hubungkan Google Drive dan buat passphrase terlebih dahulu.")
	case errors.Is(err, backup.ErrBusy):
		httpx.Error(w, http.StatusConflict, "BACKUP_BUSY", "Backup lain sedang berjalan.")
	case errors.Is(err, backup.ErrPassphraseExists):
		httpx.Error(w, http.StatusConflict, "BACKUP_PASSPHRASE_EXISTS", "Passphrase sudah dibuat dan tidak bisa ditampilkan lagi.")
	case errors.Is(err, backup.ErrNoOwnerURL), errors.Is(err, backup.ErrNotOwner):
		httpx.Error(w, http.StatusConflict, "BACKUP_OWNER_URL", err.Error())
	case errors.Is(err, backup.ErrNoBackup):
		httpx.Error(w, http.StatusConflict, "BACKUP_NONE", "Belum ada backup berhasil untuk diuji.")
	case errors.Is(err, backup.ErrDrive):
		httpx.Error(w, http.StatusBadGateway, "BACKUP_DRIVE", "Google menolak permintaan. Periksa client ID/secret dan izin akun.")
	default:
		h.internal(w, r, msg, err)
	}
}

func (h *Handler) GetBackup(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	cfg, err := h.Backup.Get(r.Context(), actor(r).ID)
	if err != nil {
		h.backupErr(w, r, "baca konfigurasi backup gagal", err)
		return
	}
	runs, err := h.Backup.ListRuns(r.Context(), 30)
	if err != nil {
		h.backupErr(w, r, "baca riwayat backup gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"config": cfg, "runs": runs})
}

type backupSaveRequest struct {
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	Enabled        bool   `json:"enabled"`
	RunHour        int    `json:"run_hour"`
	KeepCount      int    `json:"keep_count"`
	IncludeUploads bool   `json:"include_uploads"`
}

func (h *Handler) SaveBackup(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	var req backupSaveRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	err := h.Backup.Save(r.Context(), a.ID, backup.SaveInput{ClientID: req.ClientID, ClientSecret: req.ClientSecret, Enabled: req.Enabled,
		IncludeUploads: req.IncludeUploads, RunHour: req.RunHour, KeepCount: req.KeepCount})
	if err != nil {
		h.backupErr(w, r, "simpan konfigurasi backup gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionBackupSave, [16]byte{}, "", map[string]any{"enabled": req.Enabled, "run_hour": req.RunHour,
		"keep_count": req.KeepCount, "include_uploads": req.IncludeUploads, "secret_changed": req.ClientSecret != ""})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) BackupAuthURL(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	u, err := h.Backup.AuthURL(r.Context(), actor(r).ID)
	if err != nil {
		h.backupErr(w, r, "buat tautan persetujuan Google gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"url": u})
}

func (h *Handler) BackupConnect(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	email, err := h.Backup.Connect(r.Context(), a.ID, req.Code, req.State)
	if err != nil {
		h.backupErr(w, r, "sambungkan Google Drive gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionBackupConnect, [16]byte{}, "", map[string]any{"account": email})
	httpx.JSON(w, http.StatusOK, map[string]string{"account": email})
}

func (h *Handler) BackupDisconnect(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	a := actor(r)
	if err := h.Backup.Disconnect(r.Context(), a.ID); err != nil {
		h.backupErr(w, r, "putuskan Google Drive gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionBackupDisconnect, [16]byte{}, "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) BackupPassphrase(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	a := actor(r)
	p, err := h.Backup.CreatePassphrase(r.Context(), a.ID)
	if err != nil {
		h.backupErr(w, r, "buat passphrase backup gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionBackupPass, [16]byte{}, "", nil)
	httpx.JSON(w, http.StatusOK, map[string]string{"passphrase": p})
}

func (h *Handler) BackupRun(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	a := actor(r)
	id, err := h.Backup.Start(r.Context(), "manual")
	if err != nil {
		h.backupErr(w, r, "mulai backup gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionBackupRun, [16]byte{}, "", map[string]any{"run_id": id.String()})
	httpx.JSON(w, http.StatusAccepted, map[string]string{"id": id.String()})
}

func (h *Handler) BackupRestoreTest(w http.ResponseWriter, r *http.Request) {
	if h.backupOff(w) {
		return
	}
	a := actor(r)
	id, err := h.Backup.StartRestoreTest(r.Context())
	if err != nil {
		h.backupErr(w, r, "mulai uji restore gagal", err)
		return
	}
	h.svc.record(r.Context(), a, ActionBackupRestore, [16]byte{}, "", map[string]any{"run_id": id.String()})
	httpx.JSON(w, http.StatusAccepted, map[string]string{"id": id.String()})
}
