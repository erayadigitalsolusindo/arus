package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/authz"
)

const histKey = "kunci-riwayat-uji-kunci-riwayat-uji"

func histEnv(t *testing.T) *actEnv {
	t.Helper()
	e := newActEnv(t)
	if _, err := e.svc.WithHistory(histKey); err != nil {
		t.Fatal(err)
	}
	return e
}

func turn(reply string, acts ...PendingAction) *ChatResult {
	return &ChatResult{Reply: reply, Actions: acts, ToolsUsed: []ToolCall{{Name: "sales_summary"}, {Name: "sales_summary"}, {Name: "list_outlets"}}}
}

type probeResult struct{ list, get, rename, del error }

// probe mencoba semua operasi baca/ubah/hapus atas percakapan `id` sebagai `who`.
func probe(ctx context.Context, e *actEnv, who authz.Actor, id uuid.UUID) probeResult {
	var r probeResult
	_, r.list = e.svc.ListConversations(ctx, who, "", 10)
	_, r.get = e.svc.GetConversation(ctx, who, id)
	r.rename = e.svc.RenameConversation(ctx, who, id, "retas")
	r.del = e.svc.DeleteConversation(ctx, who, id)
	return r
}

func TestHistorySaveLoadEncryptedAndOwnerOnly(t *testing.T) {
	e := histEnv(t)
	ctx := context.Background()
	act := PendingAction{ID: uuid.NewString(), Kind: KindPettyAdd, Fields: map[string]string{"amount": "85000.00", "category": "Listrik"}}

	id, err := e.svc.saveTurn(ctx, e.cashier, uuid.Nil, "Omzet rahasia bulan ini berapa?", true, turn("Omzet Rp 123.456.789 (RAHASIA)", act))
	if err != nil || id == uuid.Nil {
		t.Fatalf("simpan: %v", err)
	}
	// Di DB tidak ada plaintext (judul, pertanyaan, jawaban, kartu).
	var dump string
	if err := e.admin.QueryRow(ctx, `SELECT coalesce(string_agg(c.title_enc || m.content_enc || coalesce(m.actions_enc, ''), ' '), '')
		FROM ai_conversations c JOIN ai_messages m ON m.conversation_id = c.id WHERE c.tenant_id = $1`, e.tenant).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Omzet", "RAHASIA", "123.456", "Listrik", "85000"} {
		if strings.Contains(dump, leak) {
			t.Fatalf("plaintext %q bocor di DB", leak)
		}
	}

	list, err := e.svc.ListConversations(ctx, e.cashier, "", 10)
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != id || list.Items[0].Title != "Omzet rahasia bulan ini berapa?" || list.Items[0].MsgCount != 2 {
		t.Fatalf("daftar: %+v %v", list, err)
	}
	c, err := e.svc.GetConversation(ctx, e.cashier, id)
	if err != nil || len(c.Messages) != 2 || c.Messages[0].Role != "user" || !c.Messages[0].HadImages || c.Messages[1].Content != "Omzet Rp 123.456.789 (RAHASIA)" {
		t.Fatalf("buka: %+v %v", c, err)
	}
	if got := c.Messages[1].Tools; len(got) != 2 {
		t.Fatalf("nama tool harus unik: %v", got)
	}
	if len(c.Messages[1].Actions) != 1 || c.Messages[1].Actions[0].Fields["category"] != "Listrik" || c.Messages[1].Actions[0].Status != "expired" {
		t.Fatalf("kartu yang tak hidup lagi di Redis harus kedaluwarsa: %+v", c.Messages[1].Actions)
	}
	e.svc.setActionStatus(ctx, e.cashier, act.ID, "done")
	if c, _ = e.svc.GetConversation(ctx, e.cashier, id); c.Messages[1].Actions[0].Status != "done" {
		t.Fatalf("status kartu: %+v", c.Messages[1].Actions)
	}

	// Lanjutkan percakapan: urutan pesan bertambah.
	if id2, err := e.svc.saveTurn(ctx, e.cashier, id, "Bandingkan bulan lalu", false, turn("Lebih rendah")); err != nil || id2 != id {
		t.Fatalf("lanjut: %v", err)
	}
	if c, _ = e.svc.GetConversation(ctx, e.cashier, id); len(c.Messages) != 4 || c.MsgCount != 4 || c.Messages[3].Content != "Lebih rendah" {
		t.Fatalf("setelah lanjut: %+v", c)
	}

	// Hanya pemilik: kasir lain di tenant sama, penyetuju, dan tenant lain tidak bisa melihat/mengubah/menghapus.
	otherTenant := e.cashier
	otherTenant.TenantID = uuid.New()
	for name, who := range map[string]authz.Actor{"kasir lain": e.other, "penyetuju": e.approver, "tenant lain": otherTenant} {
		r := probe(ctx, e, who, id)
		if r.get != ErrConvNotFound || r.rename != ErrConvNotFound || r.del != ErrConvNotFound || r.list != nil {
			t.Errorf("%s: %+v harus 'tidak ditemukan' untuk get/rename/del", name, r)
		}
		if l, _ := e.svc.ListConversations(ctx, who, "", 10); len(l.Items) != 0 {
			t.Errorf("%s: daftar harus kosong", name)
		}
		if _, err := e.svc.saveTurn(ctx, who, id, "x", false, turn("y")); err != ErrConvNotFound {
			t.Errorf("%s: menulis ke percakapan orang lain: %v", name, err)
		}
		if err := e.svc.checkConversation(ctx, who, id); err != ErrConvNotFound {
			t.Errorf("%s: cek percakapan orang lain: %v", name, err)
		}
	}
	// Percakapan pemilik masih utuh setelah semua percobaan itu.
	if c, err = e.svc.GetConversation(ctx, e.cashier, id); err != nil || c.Title != "Omzet rahasia bulan ini berapa?" {
		t.Fatalf("percakapan pemilik berubah: %+v %v", c, err)
	}

	// Ganti nama: judul baru dirapikan dan terbaca pemilik.
	if err := e.svc.RenameConversation(ctx, e.cashier, id, "Judul\nbaru  rapi"); err != nil {
		t.Fatal(err)
	}
	if l, _ := e.svc.ListConversations(ctx, e.cashier, "", 10); l.Items[0].Title != "Judul baru rapi" {
		t.Fatalf("judul: %q", l.Items[0].Title)
	}
}

// Ciphertext terikat ke tenant:percakapan:seq: menukar isi antar-baris terdeteksi, bukan dibaca diam-diam.
func TestHistoryTamperDetected(t *testing.T) {
	e := histEnv(t)
	ctx := context.Background()
	id, err := e.svc.saveTurn(ctx, e.cashier, uuid.Nil, "pertanyaan", false, turn("jawaban"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `UPDATE ai_messages SET content_enc = (SELECT content_enc FROM ai_messages WHERE conversation_id = $1 AND seq = 2)
		WHERE conversation_id = $1 AND seq = 1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetConversation(ctx, e.cashier, id); err == nil {
		t.Fatal("isi yang ditukar antar-pesan harus gagal dibuka")
	}
	// Kunci berbeda tidak bisa membuka riwayat.
	other, _ := NewService(e.svc.pool, e.svc.rdb, "0123456789abcdef0123456789abcdef", Free{}, e.svc.log)
	if _, err := other.WithHistory("kunci-lain-kunci-lain-kunci-lain-123"); err != nil {
		t.Fatal(err)
	}
	e2, _ := e.svc.saveTurn(ctx, e.cashier, uuid.Nil, "p2", false, turn("j2"))
	if _, err := other.GetConversation(ctx, e.cashier, e2); err == nil {
		t.Fatal("kunci riwayat berbeda tidak boleh bisa membuka")
	}
}

func TestHistoryLimitsRetentionAndPaging(t *testing.T) {
	e := histEnv(t)
	ctx := context.Background()

	// Batas 50 percakapan per pengguna: yang terlama dibuang; pengguna lain tidak terpengaruh.
	other, err := e.svc.saveTurn(ctx, e.other, uuid.Nil, "milik lain", false, turn("ok"))
	if err != nil {
		t.Fatal(err)
	}
	var first uuid.UUID
	for i := 0; i < maxConversations+2; i++ {
		id, err := e.svc.saveTurn(ctx, e.cashier, uuid.Nil, "q"+strings.Repeat("x", i), false, turn("a"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = id
		}
	}
	if n := e.count(t, `SELECT count(*) FROM ai_conversations WHERE tenant_id = $1 AND user_id = $2`, e.tenant, e.cashier.UserID); n != maxConversations {
		t.Fatalf("batas %d percakapan, got %d", maxConversations, n)
	}
	if _, err := e.svc.GetConversation(ctx, e.cashier, first); err != ErrConvNotFound {
		t.Fatalf("yang terlama harus terbuang: %v", err)
	}
	if _, err := e.svc.GetConversation(ctx, e.other, other); err != nil {
		t.Fatalf("pengguna lain tidak boleh terpengaruh: %v", err)
	}

	// Keyset: halaman 20 + 20 + 10, tanpa duplikat dan tanpa celah.
	seen := map[uuid.UUID]bool{}
	cursor, pages := "", 0
	for {
		pg, err := e.svc.ListConversations(ctx, e.cashier, cursor, 20)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, it := range pg.Items {
			if seen[it.ID] {
				t.Fatalf("duplikat di halaman %d", pages)
			}
			seen[it.ID] = true
		}
		if pg.Next == "" {
			break
		}
		cursor = pg.Next
	}
	if len(seen) != maxConversations || pages != 3 {
		t.Fatalf("keyset: %d item, %d halaman", len(seen), pages)
	}
	if _, err := e.svc.ListConversations(ctx, e.cashier, "bukan-kursor", 10); err != ErrBadInput {
		t.Fatalf("kursor rusak: %v", err)
	}

	// Retensi 90 hari: disembunyikan saat baca dan dibersihkan saat pemilik membuat percakapan baru.
	old, _ := e.svc.saveTurn(ctx, e.other, uuid.Nil, "lama", false, turn("ok"))
	if _, err := e.admin.Exec(ctx, `UPDATE ai_conversations SET updated_at = now() - interval '91 days' WHERE id = $1`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetConversation(ctx, e.other, old); err != ErrConvNotFound {
		t.Fatalf("kedaluwarsa harus tersembunyi: %v", err)
	}
	if err := e.svc.checkConversation(ctx, e.other, old); err != ErrConvNotFound {
		t.Fatalf("kedaluwarsa tak boleh dilanjutkan: %v", err)
	}
	if _, err := e.svc.saveTurn(ctx, e.other, uuid.Nil, "baru", false, turn("ok")); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_conversations WHERE id = $1`, old); n != 0 {
		t.Fatal("yang kedaluwarsa harus dibersihkan saat membuat percakapan baru")
	}

	// Batas pesan per percakapan.
	full, _ := e.svc.saveTurn(ctx, e.cashier, uuid.Nil, "penuh", false, turn("ok"))
	if _, err := e.admin.Exec(ctx, `UPDATE ai_conversations SET msg_count = $2 WHERE id = $1`, full, maxConvMessages-1); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.checkConversation(ctx, e.cashier, full); err != ErrConvFull {
		t.Fatalf("penuh: %v", err)
	}
	if _, err := e.svc.saveTurn(ctx, e.cashier, full, "x", false, turn("y")); err != ErrConvFull {
		t.Fatalf("simpan ke yang penuh: %v", err)
	}
}

func TestHistoryDeleteAllOnlyMine(t *testing.T) {
	e := histEnv(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = e.svc.saveTurn(ctx, e.cashier, uuid.Nil, "mine", false, turn("a"))
	}
	theirs, _ := e.svc.saveTurn(ctx, e.other, uuid.Nil, "theirs", false, turn("a"))
	n, err := e.svc.DeleteAllConversations(ctx, e.cashier)
	if err != nil || n != 3 {
		t.Fatalf("hapus semua: %d %v", n, err)
	}
	if _, err := e.svc.GetConversation(ctx, e.other, theirs); err != nil {
		t.Fatalf("milik orang lain harus tetap ada: %v", err)
	}
	if c := e.count(t, `SELECT count(*) FROM ai_messages WHERE tenant_id = $1`, e.tenant); c != 2 {
		t.Fatalf("pesan ikut terhapus (cascade), tersisa %d", c)
	}
	if c := e.count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'ai.history_clear'`, e.tenant); c != 1 {
		t.Fatalf("audit hapus riwayat: %d", c)
	}
}

// Alur penuh lewat ChatWith + endpoint kustom tiruan: sementara/saklar mati tidak menyimpan; normal menyimpan dan bisa dilanjutkan.
func TestChatWithHistorySwitches(t *testing.T) {
	e := histEnv(t)
	ctx := context.Background()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"jawaban uji"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()
	origClient, origResolve := newCustomClient, checkResolve
	newCustomClient = func(base string) *openaiClient {
		return &openaiClient{http: srv.Client(), base: base, label: "T", probe: true}
	}
	checkResolve = func(context.Context, string) error { return nil }
	t.Cleanup(func() { newCustomClient, checkResolve = origClient, origResolve })
	if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{Label: "T", BaseURL: srv.URL + "/v1", Model: "m-uji", APIKey: "KEY-UJI-12345"}); err != nil {
		t.Fatal(err)
	}
	msgs := []ChatMessage{{Role: "user", Content: "halo riwayat"}}
	rows := func() int { return e.count(t, `SELECT count(*) FROM ai_conversations WHERE tenant_id = $1`, e.tenant) }

	// Percakapan sementara tidak menyimpan.
	if r, err := e.svc.ChatWith(ctx, e.cashier, msgs, ChatOptions{Temporary: true}); err != nil || r.ConversationID != "" || rows() != 0 {
		t.Fatalf("sementara: %+v %v rows=%d", r, err, rows())
	}
	// Normal: tersimpan, lalu dilanjutkan dengan id yang sama.
	r1, err := e.svc.ChatWith(ctx, e.cashier, msgs, ChatOptions{})
	if err != nil || r1.ConversationID == "" || rows() != 1 {
		t.Fatalf("normal: %+v %v rows=%d", r1, err, rows())
	}
	msgs = append(msgs, ChatMessage{Role: "assistant", Content: r1.Reply}, ChatMessage{Role: "user", Content: "lanjut"})
	r2, err := e.svc.ChatWith(ctx, e.cashier, msgs, ChatOptions{ConversationID: r1.ConversationID})
	if err != nil || r2.ConversationID != r1.ConversationID || rows() != 1 {
		t.Fatalf("lanjut: %+v %v", r2, err)
	}
	cid, _ := uuid.Parse(r1.ConversationID)
	if c, err := e.svc.GetConversation(ctx, e.cashier, cid); err != nil || len(c.Messages) != 4 || c.Messages[2].Content != "lanjut" || c.Messages[3].Content != "jawaban uji" {
		t.Fatalf("isi tersimpan: %+v %v", c, err)
	}
	// Hanya pasangan terbaru yang disimpan per giliran (riwayat dari klien tidak diduplikasi); id orang lain ditolak sebelum menghabiskan token.
	if _, err := e.svc.ChatWith(ctx, e.other, msgs, ChatOptions{ConversationID: r1.ConversationID}); err != ErrConvNotFound {
		t.Fatalf("lanjutkan percakapan orang lain: %v", err)
	}
	if _, err := e.svc.ChatWith(ctx, e.cashier, msgs, ChatOptions{ConversationID: "bukan-uuid"}); err != ErrBadInput {
		t.Fatalf("id rusak: %v", err)
	}

	// Saklar tenant mati: tidak menyimpan walau tidak sementara; menyala lagi: menyimpan.
	if err := e.svc.SetHistoryEnabled(ctx, e.cashier, false); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.svc.Settings(ctx, e.cashier); st.History.Enabled || !st.History.Available {
		t.Fatalf("settings history: %+v", st.History)
	}
	if r, err := e.svc.ChatWith(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "tidak disimpan"}}, ChatOptions{}); err != nil || r.ConversationID != "" || rows() != 1 {
		t.Fatalf("saklar mati: %+v %v rows=%d", r, err, rows())
	}
	if err := e.svc.SetHistoryEnabled(ctx, e.cashier, true); err != nil {
		t.Fatal(err)
	}
	if r, err := e.svc.ChatWith(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "disimpan lagi"}}, ChatOptions{}); err != nil || r.ConversationID == "" {
		t.Fatalf("saklar nyala: %+v %v", r, err)
	}
	// Tanpa kunci riwayat (WithHistory tak dipanggil) → tidak tersedia dan tidak menyimpan.
	noKey := *e.svc
	noKey.hbox = nil
	if st, _ := noKey.Settings(ctx, e.cashier); st.History.Available || st.History.Enabled {
		t.Fatalf("tanpa kunci: %+v", st.History)
	}
	n0 := rows()
	if r, err := noKey.ChatWith(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "x"}}, ChatOptions{}); err != nil || r.ConversationID != "" || rows() != n0 {
		t.Fatalf("tanpa kunci: %+v %v", r, err)
	}
}
