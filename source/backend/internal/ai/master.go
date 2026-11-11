package ai

// Aksi tulis data MASTER lewat HalloArus: barang/harga, pelanggan, supplier, kupon, dan halaman storefront.
// Satu tool usulan generik (`propose_master_change`) + satu tool pencarian id (`lookup_master_records`).
// Aturan (jangan dilonggarkan, melengkapi prinsip di actions.go):
//   - Hanya data master. Transaksi kasir, pembelian, pembayaran hutang/piutang, koreksi stok, dan void transaksi TIDAK punya tool
//     di sini dan tidak boleh ditambahkan: AI hanya membaca dan menjelaskannya.
//   - Tidak ada hapus permanen: "hapus" = nonaktifkan (set_active=false), tetap wajib PIN + TOTP penyetuju seperti aksi lain.
//   - Satu usulan = satu data (tidak ada aksi massal).
//   - Kolom yang boleh diisi dibatasi daftar putih per entitas; nilai divalidasi server dan ringkasan kartu dibuat server dari data
//     saat ini (sebelum → sesudah). Model hanya diberi tahu NAMA kolom yang berubah, bukan nilai lama (data pelanggan tidak
//     dikirim ke AI).
//   - Eksekusi memakai service resmi dengan hak pengguna (izin modul, audit, batas paket) dan memuat ulang data saat disetujui.

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/aipolicy"
	"aciraba/internal/authz"
	"aciraba/internal/catalog"
	"aciraba/internal/item"
	"aciraba/internal/member"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/storefront"
	"aciraba/internal/voucher"
)

const (
	KindMaster = "master_change"

	ToolProposeMaster = "propose_master_change"
	ToolLookupMaster  = "lookup_master_records"

	lookupMax = 8
)

// Masters = service data master yang boleh diubah AI. Nil = entitas itu tidak tersedia.
type Masters struct {
	Items      *item.Service
	Members    *member.Service
	Catalog    *catalog.Service
	Vouchers   *voucher.Service
	Storefront *storefront.Service
}

// WithMasters mengaktifkan tool tulis data master.
func (s *Service) WithMasters(m Masters) *Service {
	s.masters = m
	return s
}

// MasterError = galat validasi/aturan dari service master, diteruskan apa adanya ke kartu (bukan 500).
type MasterError struct {
	Status int
	Code   string
	Msg    string
	Fields map[string]string
}

func (e *MasterError) Error() string { return e.Msg }

// wrapMasterErr mengubah galat service master menjadi MasterError; galat lain dikembalikan apa adanya.
func wrapMasterErr(err error) error {
	if err == nil {
		return nil
	}
	var fi item.FieldErrors
	var fm member.FieldErrors
	var fc catalog.FieldErrors
	var fv voucher.FieldErrors
	var fs storefront.FieldErrors
	switch {
	case errors.As(err, &fi):
		return &MasterError{Status: 422, Code: "VALIDATION", Msg: "Data tidak valid.", Fields: fi}
	case errors.As(err, &fm):
		return &MasterError{Status: 422, Code: "VALIDATION", Msg: "Data tidak valid.", Fields: fm}
	case errors.As(err, &fc):
		return &MasterError{Status: 422, Code: "VALIDATION", Msg: "Data tidak valid.", Fields: fc}
	case errors.As(err, &fv):
		return &MasterError{Status: 422, Code: "VALIDATION", Msg: "Data tidak valid.", Fields: fv}
	case errors.As(err, &fs):
		return &MasterError{Status: 422, Code: "VALIDATION", Msg: "Data tidak valid.", Fields: fs}
	case errors.Is(err, item.ErrCodeTaken), errors.Is(err, member.ErrCodeTaken), errors.Is(err, catalog.ErrCodeTaken),
		errors.Is(err, catalog.ErrNameTaken), errors.Is(err, voucher.ErrCodeTaken), errors.Is(err, storefront.ErrSlugTaken):
		return &MasterError{Status: 409, Code: "CODE_TAKEN", Msg: "Kode atau nama sudah dipakai."}
	case errors.Is(err, item.ErrNotFound), errors.Is(err, member.ErrNotFound), errors.Is(err, catalog.ErrNotFound),
		errors.Is(err, voucher.ErrNotFound), errors.Is(err, storefront.ErrNotFound):
		return &MasterError{Status: 404, Code: "NOT_FOUND", Msg: "Data tidak ditemukan."}
	}
	return err
}

// ---- jenis nilai kolom ----

const (
	vName  = "name"
	vText  = "text"
	vMoney = "money"
	vInt   = "int"
	vQty   = "qty"
	vBool  = "bool"
	vDate  = "date"
	vEnum  = "enum"
	vCode  = "code" // HTML/CSS halaman: apa adanya (disaring storefront.Save), maks 60 rb karakter
)

var fieldKind = map[string]string{
	"name": vName, "sku": vName, "barcode": vName, "unit": vName, "code": vName, "contact_name": vName, "phone": vName, "email": vName,
	"city": vName, "district": vName, "province": vName, "postal_code": vName, "gender": vEnum, "title": vName,
	"description": vText, "note": vText, "notes": vText, "address": vText, "find": vText, "replace": vText,
	"sell_price": vMoney, "cost": vMoney, "credit_limit": vMoney, "value": vMoney, "max_discount": vMoney, "min_spend": vMoney,
	"min_stock": vQty, "due_days": vInt, "max_uses": vInt, "enabled": vBool, "valid_until": vDate, "starts_on": vDate, "ends_on": vDate,
	"kind": vEnum, "slug": vName, "html": vCode, "css": vCode,
}

var enumValues = map[string][]string{
	"gender": {"male", "female", ""},
	"kind":   {"percent", "fixed"},
}

const maxMasterMoney = 1_000_000_000_000

// normField memvalidasi + menormalkan satu nilai dari model. "" diizinkan (mengosongkan kolom opsional).
func normField(key, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	bad := fmt.Errorf("nilai kolom %q tidak valid", key)
	switch fieldKind[key] {
	case vName:
		if raw == "" {
			return "", nil
		}
		v, code := sanitize.Name(raw, 150)
		if code != "" {
			return "", bad
		}
		return v, nil
	case vCode:
		if len(raw) > 60000 {
			return "", bad
		}
		return raw, nil
	case vText:
		if raw == "" {
			return "", nil
		}
		v, code := sanitize.Multiline(raw, 600)
		if code != "" {
			return "", bad
		}
		return v, nil
	case vMoney:
		if raw == "" {
			return "", nil
		}
		d, err := decimal.NewFromString(raw)
		if err != nil || d.IsNegative() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(decimal.NewFromInt(maxMasterMoney)) {
			return "", fmt.Errorf("kolom %q harus angka rupiah >= 0 (maks 2 desimal, tanpa pemisah ribuan)", key)
		}
		return d.StringFixed(2), nil
	case vQty:
		if raw == "" {
			return "", nil
		}
		d, err := decimal.NewFromString(raw)
		if err != nil || d.IsNegative() || !d.Equal(d.Round(3)) || d.GreaterThanOrEqual(decimal.NewFromInt(maxMasterMoney)) {
			return "", fmt.Errorf("kolom %q harus angka >= 0 (maks 3 desimal)", key)
		}
		return d.String(), nil
	case vInt:
		if raw == "" {
			return "", nil
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 1_000_000 {
			return "", fmt.Errorf("kolom %q harus bilangan bulat >= 0", key)
		}
		return strconv.Itoa(n), nil
	case vBool:
		if raw != "true" && raw != "false" {
			return "", fmt.Errorf("kolom %q harus true atau false", key)
		}
		return raw, nil
	case vDate:
		if raw == "" {
			return "", nil
		}
		if len(raw) != 10 || raw[4] != '-' || raw[7] != '-' {
			return "", fmt.Errorf("kolom %q harus berformat YYYY-MM-DD", key)
		}
		return raw, nil
	case vEnum:
		if !slices.Contains(enumValues[key], raw) {
			return "", fmt.Errorf("kolom %q harus salah satu dari: %s", key, strings.Join(enumValues[key], ", "))
		}
		return raw, nil
	}
	return "", fmt.Errorf("kolom %q tidak dikenal", key)
}

// ---- entitas ----

type masterEntity struct {
	module                string
	domain                string // domain kebijakan baca (aipolicy) untuk pencarian id; "" = tak terikat
	createKeys            []string
	updateKeys            []string
	required              []string
	noID                  bool // satu data per tenant (storefront): tanpa id; `update`, dan `create` bila `create` diisi (hanya saat belum ada)
	lookup                func(ctx context.Context, a authz.Actor, q string) ([]map[string]string, error)
	show                  func(ctx context.Context, a authz.Actor, id uuid.UUID) (target string, cur map[string]string, active bool, err error)
	create                func(ctx context.Context, a authz.Actor, f map[string]string) error
	update                func(ctx context.Context, a authz.Actor, id uuid.UUID, f map[string]string) error
	setActive             func(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) error
	opLabel, lookupFields string
}

func (s *Service) entities() map[string]masterEntity {
	m := s.masters
	out := map[string]masterEntity{}
	if m.Items != nil {
		out["item"] = s.itemEntity(m.Items)
	}
	if m.Members != nil {
		out["member"] = s.memberEntity(m.Members)
	}
	if m.Catalog != nil {
		out["supplier"] = s.supplierEntity(m.Catalog)
	}
	if m.Vouchers != nil {
		out["voucher"] = s.voucherEntity(m.Vouchers)
	}
	if m.Storefront != nil {
		out["storefront"] = s.storefrontEntity(m.Storefront)
	}
	return out
}

func entityNames(m map[string]masterEntity) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func masterToolDefs(ents map[string]masterEntity) []toolDef {
	names := entityNames(ents)
	return []toolDef{
		{
			Name: ToolProposeMaster,
			Description: "Propose creating, editing or deactivating ONE master-data record (item/price, member/customer, supplier, voucher, or the storefront page). " +
				"This does NOT save anything: it creates a proposal card; an approver must type their PIN and 2FA code on screen. Never ask for either in chat. " +
				"One record per proposal; no bulk. There is NO delete: deactivating (op=set_active, active=false) is the way to remove something. " +
				"You can NOT create or change sales, purchases, stock, debt or receivable payments: tell the user to use the regular screens. " +
				"Never invent values; ask when unsure. Use lookup_master_records first to get the id for update/set_active. " +
				"Allowed fields — item: create{name,sku,barcode,sell_price,cost,min_stock,description,unit(name of an existing unit)} update{name,barcode,sell_price,min_stock,description}; " +
				"member: {code,name,gender(male|female),phone,email,address,district,city,province,postal_code,credit_limit,due_days,valid_until,notes}; " +
				"supplier: {code,name,contact_name,phone,email,address,note}; " +
				"voucher: {code,name,kind(percent|fixed),value,max_discount,min_spend,starts_on,ends_on,max_uses}; " +
				"storefront (no id; one landing page per tenant): create{slug(3-40 lowercase letters/digits/dashes),title,html,css} only when none exists yet — saved DISABLED, publish with a later update enabled=true; " +
				"update{title, enabled(true|false), html and/or css = REPLACE the whole page (you cannot read the current html, so only when the user wants a full redesign; the old version stays in version history), find+replace = replace ONE exact plain-text phrase in the page}. " +
				"Landing page html: body-level HTML only, Tailwind utility classes, NO script/style/form/iframe/on* attributes, links http(s)/mailto/tel only, css optional and must not use url()/@import. " +
				"Markers (filled by the server): {namatoko} {tahun} {kategori1..} {namabarang} {harga} {gambar}; loops {#barang kategori=1 limit=8 urut=terbaru}…{/barang} and {#kategori limit=6}{namakategori}…{/kategori}. The domain/host is set by the platform operator, not you. " +
				"Money = digits with optional 2 decimals, no thousand separators; dates = YYYY-MM-DD.",
			InputSchema: schema(map[string]any{
				"entity": enum("Which master data.", names...),
				"op":     enum("`create`, `update`, or `set_active` (activate/deactivate).", "create", "update", "set_active"),
				"id":     map[string]any{"type": "string", "description": "Record id (uuid) from lookup_master_records; required for update/set_active (not for storefront)."},
				"fields": map[string]any{"type": "object", "description": "Field values for create/update (see allowed fields).", "additionalProperties": map[string]any{"type": []string{"string", "number", "boolean"}}},
				"active": map[string]any{"type": "boolean", "description": "For set_active: true = activate, false = deactivate."},
			}, "entity", "op"),
		},
		{
			Name: ToolLookupMaster,
			Description: "Find master records by name/code to get their ids (max 8): item (id, sku, name), member (id, code, name), supplier (id, name), voucher (id, code, name). " +
				"Use before propose_master_change for update/set_active. Returns no other personal data.",
			InputSchema: schema(map[string]any{
				"entity": enum("Which master data.", slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == "storefront" })...),
				"q":      map[string]any{"type": "string", "description": "Name or code to search (partial match)."},
			}, "entity", "q"),
		},
	}
}

func isMasterTool(name string) bool { return name == ToolProposeMaster }
func isLookupTool(name string) bool { return name == ToolLookupMaster }

// anyMasterAllowed = pengguna ini diberi tool master (semua entitas yang ada, dikurangi yang tak berizin ditolak saat usulan).
func (s *Service) masterEnabled() bool {
	m := s.masters
	return m.Items != nil || m.Members != nil || m.Catalog != nil || m.Vouchers != nil || m.Storefront != nil
}

func (e masterEntity) allowed(a authz.Actor, op string) bool {
	switch op {
	case "create":
		if e.noID { // satu data per tenant: membuat = menyimpan pertama kali, izinnya sama dengan mengubah
			return e.create != nil && a.Perms.Has(e.module, authz.ActUpdate)
		}
		return a.Perms.Has(e.module, authz.ActCreate)
	default:
		return a.Perms.Has(e.module, authz.ActUpdate)
	}
}

// fieldMap mengubah `fields` dari model menjadi peta string tervalidasi dan hanya berisi kolom yang diizinkan.
func fieldMap(p params, allowed []string) (map[string]string, error) {
	raw, _ := p["fields"].(map[string]any)
	out := map[string]string{}
	for k, v := range raw {
		if !slices.Contains(allowed, k) {
			return nil, fmt.Errorf("kolom %q tidak diizinkan; kolom yang boleh: %s", k, strings.Join(allowed, ", "))
		}
		var s string
		switch x := v.(type) {
		case string:
			s = x
		case float64:
			s = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			s = strconv.FormatBool(x)
		case nil:
		default:
			return nil, fmt.Errorf("nilai kolom %q harus teks, angka, atau true/false", k)
		}
		n, err := normField(k, s)
		if err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, nil
}

type masterPayload struct {
	Entity string            `json:"entity"`
	Op     string            `json:"op"`
	ID     uuid.UUID         `json:"id"`
	Fields map[string]string `json:"fields"`
	Active *bool             `json:"active"`
}

func yesNo(b bool) string {
	if b {
		return "aktif"
	}
	return "nonaktif"
}

// proposeMaster memvalidasi usulan dan menyusun kartu. Hasil kedua = ringkasan untuk model (tanpa nilai lama).
func (s *Service) proposeMaster(ctx context.Context, a authz.Actor, p params) (storedAction, map[string]any, error) {
	ents := s.entities()
	name, err := p.oneOf("entity", "", entityNames(ents)...)
	if err != nil || name == "" {
		return storedAction{}, nil, errors.New("`entity` wajib: " + strings.Join(entityNames(ents), ", "))
	}
	ent := ents[name]
	op, err := p.oneOf("op", "", "create", "update", "set_active")
	if err != nil || op == "" {
		return storedAction{}, nil, errors.New("`op` wajib: create, update, atau set_active")
	}
	if ent.noID && op != "update" && (op != "create" || ent.create == nil) {
		return storedAction{}, nil, fmt.Errorf("%s hanya mendukung op=update atau create", name)
	}
	if op == "set_active" && ent.setActive == nil {
		return storedAction{}, nil, fmt.Errorf("%s tidak bisa diaktifkan/dinonaktifkan", name)
	}
	if !ent.allowed(a, op) {
		return storedAction{}, nil, errors.New("pengguna tidak punya izin untuk perubahan ini")
	}
	if err := s.domainOpen(ctx, a, ent); err != nil {
		return storedAction{}, nil, err
	}
	pl := masterPayload{Entity: name, Op: op}
	fields := map[string]string{"entity": name, "op": op}
	changed := []string{}

	if op != "create" && !ent.noID {
		id, err := uuid.Parse(p.str("id"))
		if err != nil {
			return storedAction{}, nil, errors.New("`id` bukan uuid; ambil dari lookup_master_records")
		}
		pl.ID = id
	}

	switch op {
	case "create":
		f, err := fieldMap(p, ent.createKeys)
		if err != nil {
			return storedAction{}, nil, err
		}
		for _, r := range ent.required {
			if f[r] == "" {
				return storedAction{}, nil, fmt.Errorf("kolom %q wajib untuk membuat data baru", r)
			}
		}
		if ent.noID {
			if _, _, _, err := ent.show(ctx, a, uuid.Nil); err == nil {
				return storedAction{}, nil, fmt.Errorf("%s sudah ada; pakai op=update", name)
			}
		}
		pl.Fields = f
		fields["target"] = cmp.Or(f["name"], f["title"], f["slug"])
		for _, k := range sortedKeys(f) {
			if f[k] != "" {
				fields["chg."+k] = codeSummary(k, f[k])
				changed = append(changed, k)
			}
		}
	case "update":
		f, err := fieldMap(p, ent.updateKeys)
		if err != nil {
			return storedAction{}, nil, err
		}
		if len(f) == 0 {
			return storedAction{}, nil, errors.New("`fields` kosong: tidak ada yang diubah")
		}
		target, cur, _, err := ent.show(ctx, a, pl.ID)
		if err != nil {
			return storedAction{}, nil, wrapShowErr(err)
		}
		pl.Fields = f
		fields["target"] = target
		for _, k := range sortedKeys(f) {
			if cur[k] == f[k] {
				continue
			}
			fields["chg."+k] = displayVal(codeSummary(k, cur[k])) + " → " + displayVal(codeSummary(k, f[k]))
			changed = append(changed, k)
		}
		if len(changed) == 0 {
			return storedAction{}, nil, errors.New("nilai yang diusulkan sama dengan data saat ini; tidak ada yang berubah")
		}
	case "set_active":
		v, ok := p["active"].(bool)
		if !ok {
			return storedAction{}, nil, errors.New("`active` (true/false) wajib untuk set_active")
		}
		target, _, cur, err := ent.show(ctx, a, pl.ID)
		if err != nil {
			return storedAction{}, nil, wrapShowErr(err)
		}
		if cur == v {
			return storedAction{}, nil, fmt.Errorf("data ini sudah %s", yesNo(v))
		}
		pl.Active = &v
		fields["target"] = target
		fields["chg.active"] = yesNo(cur) + " → " + yesNo(v)
		changed = append(changed, "active")
	}
	payload, _ := json.Marshal(pl)
	st := storedAction{PendingAction: PendingAction{Kind: KindMaster, Fields: fields}, Payload: payload}
	target := fields["target"]
	if name == "member" {
		target = "(pelanggan terpilih)" // nama pelanggan tidak dikirim ke model
	}
	return st, map[string]any{"entity": name, "op": op, "target": target, "changed_fields": changed}, nil
}

// codeSummary: HTML/CSS tidak ditampilkan utuh di kartu persetujuan (puluhan ribu karakter).
func codeSummary(k, v string) string {
	if fieldKind[k] == vCode {
		return fmt.Sprintf("(%d karakter)", len(v))
	}
	return v
}

func displayVal(s string) string {
	if s == "" {
		return "(kosong)"
	}
	return s
}

func wrapShowErr(err error) error {
	err = wrapMasterErr(err)
	var me *MasterError
	if errors.As(err, &me) && me.Status == 404 {
		return errors.New("data tidak ditemukan; periksa id lewat lookup_master_records")
	}
	return err
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// execMaster menjalankan usulan master yang sudah disetujui lewat service resmi.
func (s *Service) execMaster(ctx context.Context, a authz.Actor, raw json.RawMessage) error {
	var pl masterPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return ErrActionNotFound
	}
	ent, ok := s.entities()[pl.Entity]
	if !ok || !ent.allowed(a, pl.Op) {
		return ErrActionForbid
	}
	if err := s.domainOpen(ctx, a, ent); err != nil {
		return ErrActionForbid
	}
	var err error
	switch pl.Op {
	case "create":
		err = ent.create(ctx, a, pl.Fields)
	case "update":
		err = ent.update(ctx, a, pl.ID, pl.Fields)
	case "set_active":
		if pl.Active == nil || ent.setActive == nil {
			return ErrActionNotFound
		}
		err = ent.setActive(ctx, a, pl.ID, *pl.Active)
	default:
		return ErrActionNotFound
	}
	return wrapMasterErr(err)
}

// domainOpen: menulis data master mengikuti domain kebijakan platform yang sama dengan membacanya; domain yang dimatikan operator
// platform untuk tenant ini juga menutup tulis ke entitas itu.
func (s *Service) domainOpen(ctx context.Context, a authz.Actor, ent masterEntity) error {
	if ent.domain == "" {
		return nil
	}
	pol, err := s.mustPolicy(ctx, a)
	if err != nil {
		return err
	}
	if !slices.Contains(pol.Domains, ent.domain) {
		return errors.New("data ini dinonaktifkan oleh operator platform")
	}
	return nil
}

// lookupMaster = tool baca pencarian id; hanya id + nama/kode, dan hanya untuk entitas yang boleh dibaca pengguna.
func (s *Service) lookupMaster(ctx context.Context, a authz.Actor, pol aipolicy.Effective, raw json.RawMessage) (string, error) {
	if !s.writeAllowed(ctx, a) {
		return "", errors.New("pengguna tidak punya izin menjalankan aksi tulis lewat AI")
	}
	p := params{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", errors.New("parameter bukan JSON objek yang valid")
		}
	}
	ents := s.entities()
	name, err := p.oneOf("entity", "", entityNames(ents)...)
	if err != nil || name == "" || name == "storefront" {
		return "", errors.New("`entity` wajib: item, member, supplier, atau voucher")
	}
	ent := ents[name]
	if ent.domain != "" && !slices.Contains(pol.Domains, ent.domain) {
		return "", errors.New("data ini dinonaktifkan oleh operator platform")
	}
	if !a.Perms.Has(ent.module, authz.ActView) {
		return "", errors.New("pengguna tidak punya izin melihat data ini")
	}
	q := p.str("q")
	if q == "" {
		return "", errors.New("`q` wajib diisi")
	}
	rows, err := ent.lookup(ctx, a, q)
	if err != nil {
		return "", wrapMasterErr(err)
	}
	out, _ := json.Marshal(map[string]any{"matches": rows, "note": "max 8; refine q if the right record is missing"})
	return string(out), nil
}

// ---- barang ----

func (s *Service) itemEntity(svc *item.Service) masterEntity {
	str := func(it *item.Item) map[string]string {
		return map[string]string{"name": it.Name, "barcode": it.Barcode, "sell_price": it.SellPrice, "min_stock": it.MinStock, "description": it.Description}
	}
	return masterEntity{
		module: "items", domain: aipolicy.DomainStock,
		createKeys: []string{"name", "sku", "barcode", "sell_price", "cost", "min_stock", "description", "unit"},
		updateKeys: []string{"name", "barcode", "sell_price", "min_stock", "description"},
		required:   []string{"name", "sell_price", "unit"},
		lookup: func(ctx context.Context, a authz.Actor, q string) ([]map[string]string, error) {
			rows, _, err := svc.List(ctx, a, item.ListParams{Q: q, Limit: lookupMax})
			out := []map[string]string{}
			for _, r := range rows {
				out = append(out, map[string]string{"id": r.ID.String(), "sku": r.SKU, "name": r.Name, "price": r.Price, "active": strconv.FormatBool(r.Active)})
			}
			return out, err
		},
		show: func(ctx context.Context, a authz.Actor, id uuid.UUID) (string, map[string]string, bool, error) {
			it, err := svc.Get(ctx, a, id)
			if err != nil {
				return "", nil, false, err
			}
			cur := str(it)
			if d, err := decimal.NewFromString(cur["sell_price"]); err == nil {
				cur["sell_price"] = d.StringFixed(2)
			}
			return it.Name, cur, it.Active, nil
		},
		create: func(ctx context.Context, a authz.Actor, f map[string]string) error {
			var unit uuid.UUID
			err := s.readTx(ctx, a, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT id FROM units WHERE tenant_id = $1 AND active AND lower(name) = lower($2) LIMIT 1`, a.TenantID, f["unit"]).Scan(&unit)
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return &MasterError{Status: 422, Code: "VALIDATION", Msg: "Satuan tidak ditemukan.", Fields: map[string]string{"unit": "INVALID"}}
			}
			if err != nil {
				return err
			}
			zero := func(v string) string {
				if v == "" {
					return "0"
				}
				return v
			}
			_, err = svc.Create(ctx, a, item.Input{
				SKU: f["sku"], Barcode: f["barcode"], Name: f["name"], Weight: "0", MinStock: zero(f["min_stock"]), Cost: zero(f["cost"]),
				SellPrice: f["sell_price"], UnitID: unit.String(), Kind: "goods", Description: f["description"],
			})
			return err
		},
		update: func(ctx context.Context, a authz.Actor, id uuid.UUID, f map[string]string) error {
			it, err := svc.Get(ctx, a, id)
			if err != nil {
				return err
			}
			in := item.Input{
				SKU: it.SKU, Barcode: it.Barcode, Origin: it.Origin, Name: it.Name, Weight: it.WeightGrams, MinStock: it.MinStock, Cost: it.LastCost,
				SellPrice: it.SellPrice, UnitID: it.Unit.ID.String(), Kind: it.Kind, AllowNegativeStock: it.AllowNegativeStock,
				SellBelowCost: it.SellBelowCost, Description: it.Description,
			}
			for _, r := range []struct {
				ref *item.Ref
				dst *string
			}{{it.Category, &in.CategoryID}, {it.Brand, &in.BrandID}, {it.Principal, &in.PrincipalID}, {it.Supplier, &in.SupplierID}} {
				if r.ref != nil {
					*r.dst = r.ref.ID.String()
				}
			}
			for k, v := range f {
				switch k {
				case "name":
					in.Name = v
				case "barcode":
					in.Barcode = v
				case "sell_price":
					in.SellPrice = v
				case "min_stock":
					in.MinStock = v
				case "description":
					in.Description = v
				}
			}
			_, err = svc.Update(ctx, a, id, in)
			return err
		},
		setActive: func(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) error {
			_, err := svc.SetActive(ctx, a, id, active)
			return err
		},
	}
}

// ---- pelanggan ----

func (s *Service) memberEntity(svc *member.Service) masterEntity {
	keys := []string{"code", "name", "gender", "phone", "email", "address", "district", "city", "province", "postal_code", "credit_limit", "due_days", "valid_until", "notes"}
	cur := func(m *member.Member) map[string]string {
		vu := ""
		if m.ValidUntil != nil {
			vu = *m.ValidUntil
		}
		cl := m.CreditLimit
		if d, err := decimal.NewFromString(cl); err == nil {
			cl = d.StringFixed(2)
		}
		return map[string]string{"code": m.Code, "name": m.Name, "gender": m.Gender, "phone": m.Phone, "email": m.Email, "address": m.Address,
			"district": m.District, "city": m.City, "province": m.Province, "postal_code": m.PostalCode, "credit_limit": cl,
			"due_days": strconv.Itoa(m.DueDays), "valid_until": vu, "notes": m.Notes}
	}
	input := func(c map[string]string) member.Input {
		return member.Input{Code: c["code"], Name: c["name"], Gender: c["gender"], Phone: c["phone"], Email: c["email"], Address: c["address"],
			District: c["district"], City: c["city"], Province: c["province"], PostalCode: c["postal_code"], CreditLimit: c["credit_limit"],
			DueDays: c["due_days"], ValidUntil: c["valid_until"], Notes: c["notes"]}
	}
	return masterEntity{
		module: "members", domain: aipolicy.DomainMembers,
		createKeys: keys, updateKeys: keys, required: []string{"name"},
		lookup: func(ctx context.Context, a authz.Actor, q string) ([]map[string]string, error) {
			rows, _, err := svc.List(ctx, a, member.ListParams{Q: q, Limit: lookupMax})
			out := []map[string]string{}
			for _, r := range rows {
				out = append(out, map[string]string{"id": r.ID.String(), "code": r.Code, "name": r.Name, "active": strconv.FormatBool(r.Active)})
			}
			return out, err
		},
		show: func(ctx context.Context, a authz.Actor, id uuid.UUID) (string, map[string]string, bool, error) {
			m, err := svc.Get(ctx, a, id)
			if err != nil {
				return "", nil, false, err
			}
			return m.Name, cur(m), m.Active, nil
		},
		create: func(ctx context.Context, a authz.Actor, f map[string]string) error {
			_, err := svc.Create(ctx, a, input(f))
			return err
		},
		update: func(ctx context.Context, a authz.Actor, id uuid.UUID, f map[string]string) error {
			m, err := svc.Get(ctx, a, id)
			if err != nil {
				return err
			}
			c := cur(m)
			for k, v := range f {
				c[k] = v
			}
			_, err = svc.Update(ctx, a, id, input(c))
			return err
		},
		setActive: func(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) error {
			_, err := svc.SetActive(ctx, a, id, active)
			return err
		},
	}
}

// ---- supplier ----

func (s *Service) supplierEntity(svc *catalog.Service) masterEntity {
	keys := []string{"code", "name", "contact_name", "phone", "email", "address", "note"}
	current := func(ctx context.Context, a authz.Actor, id uuid.UUID) (map[string]string, bool, error) {
		c := map[string]string{}
		var active bool
		err := s.readTx(ctx, a, func(tx pgx.Tx) error {
			var code, name, contact, phone, email, addr, note string
			if err := tx.QueryRow(ctx, `SELECT coalesce(code, ''), name, contact_name, phone, email, address, note, active FROM suppliers WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, id).Scan(&code, &name, &contact, &phone, &email, &addr, &note, &active); err != nil {
				return err
			}
			c = map[string]string{"code": code, "name": name, "contact_name": contact, "phone": phone, "email": email, "address": addr, "note": note}
			return nil
		})
		if errors.Is(err, pgx.ErrNoRows) {
			err = catalog.ErrNotFound
		}
		return c, active, err
	}
	input := func(c map[string]string) catalog.SupplierInput {
		return catalog.SupplierInput{Code: c["code"], Name: c["name"], ContactName: c["contact_name"], Phone: c["phone"], Email: c["email"], Address: c["address"], Note: c["note"]}
	}
	return masterEntity{
		module: catalog.SupplierModule, domain: aipolicy.DomainPurchases,
		createKeys: keys, updateKeys: keys, required: []string{"name"},
		lookup: func(ctx context.Context, a authz.Actor, q string) ([]map[string]string, error) {
			rows, _, err := svc.ListSuppliers(ctx, a, catalog.ListParams{Q: q, Limit: lookupMax})
			out := []map[string]string{}
			for _, r := range rows {
				out = append(out, map[string]string{"id": r.ID.String(), "code": r.Code, "name": r.Name, "active": strconv.FormatBool(r.Active)})
			}
			return out, err
		},
		show: func(ctx context.Context, a authz.Actor, id uuid.UUID) (string, map[string]string, bool, error) {
			c, active, err := current(ctx, a, id)
			return c["name"], c, active, err
		},
		create: func(ctx context.Context, a authz.Actor, f map[string]string) error {
			_, err := svc.CreateSupplier(ctx, a, input(f))
			return err
		},
		update: func(ctx context.Context, a authz.Actor, id uuid.UUID, f map[string]string) error {
			c, _, err := current(ctx, a, id)
			if err != nil {
				return err
			}
			for k, v := range f {
				c[k] = v
			}
			_, err = svc.UpdateSupplier(ctx, a, id, input(c))
			return err
		},
		setActive: func(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) error {
			_, err := svc.SetSupplierActive(ctx, a, id, active)
			return err
		},
	}
}

// ---- kupon ----

func (s *Service) voucherEntity(svc *voucher.Service) masterEntity {
	keys := []string{"code", "name", "kind", "value", "max_discount", "min_spend", "starts_on", "ends_on", "max_uses"}
	current := func(ctx context.Context, a authz.Actor, id uuid.UUID) (map[string]string, bool, error) {
		c := map[string]string{}
		var active bool
		err := s.readTx(ctx, a, func(tx pgx.Tx) error {
			var code, name, kind, value, maxd, minSpend, starts, ends string
			var maxUses *int32
			if err := tx.QueryRow(ctx, `SELECT code, name, kind, value::text, coalesce(max_discount::text, ''), min_spend::text,
					coalesce(starts_on::text, ''), coalesce(ends_on::text, ''), max_uses, active FROM vouchers WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, id).Scan(&code, &name, &kind, &value, &maxd, &minSpend, &starts, &ends, &maxUses, &active); err != nil {
				return err
			}
			mu := ""
			if maxUses != nil {
				mu = strconv.Itoa(int(*maxUses))
			}
			norm := func(v string) string {
				if d, err := decimal.NewFromString(v); err == nil {
					return d.StringFixed(2)
				}
				return v
			}
			c = map[string]string{"code": code, "name": name, "kind": kind, "value": norm(value), "max_discount": norm(maxd), "min_spend": norm(minSpend),
				"starts_on": starts, "ends_on": ends, "max_uses": mu}
			if maxd == "" {
				c["max_discount"] = ""
			}
			return nil
		})
		if errors.Is(err, pgx.ErrNoRows) {
			err = voucher.ErrNotFound
		}
		return c, active, err
	}
	input := func(c map[string]string) voucher.Input {
		in := voucher.Input{Code: c["code"], Name: c["name"], Kind: c["kind"], Value: json.Number(c["value"]), MaxDiscount: json.Number(c["max_discount"]),
			MinSpend: json.Number(c["min_spend"]), StartsOn: c["starts_on"], EndsOn: c["ends_on"]}
		if n, err := strconv.Atoi(c["max_uses"]); err == nil {
			v := int32(n)
			in.MaxUses = &v
		}
		return in
	}
	return masterEntity{
		module: voucher.ModuleID, domain: aipolicy.DomainSales,
		createKeys: keys, updateKeys: keys, required: []string{"code", "name", "kind", "value"},
		lookup: func(ctx context.Context, a authz.Actor, q string) ([]map[string]string, error) {
			rows, _, err := svc.List(ctx, a, voucher.ListParams{Q: q, Limit: lookupMax})
			out := []map[string]string{}
			for _, r := range rows {
				out = append(out, map[string]string{"id": r.ID.String(), "code": r.Code, "name": r.Name, "active": strconv.FormatBool(r.Active)})
			}
			return out, err
		},
		show: func(ctx context.Context, a authz.Actor, id uuid.UUID) (string, map[string]string, bool, error) {
			c, active, err := current(ctx, a, id)
			return c["name"], c, active, err
		},
		create: func(ctx context.Context, a authz.Actor, f map[string]string) error {
			_, err := svc.Create(ctx, a, input(f))
			return err
		},
		update: func(ctx context.Context, a authz.Actor, id uuid.UUID, f map[string]string) error {
			c, _, err := current(ctx, a, id)
			if err != nil {
				return err
			}
			for k, v := range f {
				c[k] = v
			}
			_, err = svc.Update(ctx, a, id, input(c))
			return err
		},
		setActive: func(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) error {
			_, err := svc.SetActive(ctx, a, id, active)
			return err
		},
	}
}

// ---- storefront ----

// Alamat halaman (slug), HTML mentah, dan CSS tidak bisa diganti bebas oleh AI: hanya judul, saklar tayang, dan satu penggantian
// teks persis (find → replace) yang harus ada tepat sekali di halaman. Penyimpanan tetap lewat Save (sanitasi HTML + versi).
func (s *Service) storefrontEntity(svc *storefront.Service) masterEntity {
	keys := []string{"title", "enabled", "html", "css", "find", "replace"}
	return masterEntity{
		module: "storefront", noID: true, updateKeys: keys,
		createKeys: []string{"slug", "title", "html", "css"}, required: []string{"slug", "html"},
		// Halaman pertama selalu dibuat nonaktif; menayangkannya = usulan update enabled=true tersendiri.
		create: func(ctx context.Context, a authz.Actor, f map[string]string) error {
			if c, err := svc.Get(ctx, a); err != nil {
				return err
			} else if c != nil {
				return &MasterError{Status: 409, Code: "CONFLICT", Msg: "Landing page sudah ada; ubah lewat op=update."}
			}
			_, _, err := svc.Save(ctx, a, storefront.SaveInput{Slug: f["slug"], Title: f["title"], HTML: f["html"], CSS: f["css"]})
			return err
		},
		show: func(ctx context.Context, a authz.Actor, _ uuid.UUID) (string, map[string]string, bool, error) {
			c, err := svc.Get(ctx, a)
			if err != nil {
				return "", nil, false, err
			}
			if c == nil {
				return "", nil, false, errors.New("storefront belum dibuat; buat dulu lewat menu Storefront")
			}
			return c.Title, map[string]string{"title": c.Title, "enabled": strconv.FormatBool(c.Enabled), "html": c.HTML, "css": c.CSS, "find": "", "replace": ""}, c.Enabled, nil
		},
		update: func(ctx context.Context, a authz.Actor, _ uuid.UUID, f map[string]string) error {
			c, err := svc.Get(ctx, a)
			if err != nil {
				return err
			}
			if c == nil {
				return errors.New("storefront belum dibuat; buat dulu lewat menu Storefront")
			}
			in := storefront.SaveInput{Slug: c.Slug, Enabled: c.Enabled, Title: c.Title, HTML: c.HTML, CSS: c.CSS}
			if v, ok := f["title"]; ok {
				in.Title = v
			}
			if v, ok := f["enabled"]; ok {
				in.Enabled = v == "true"
			}
			if v, ok := f["html"]; ok {
				in.HTML = v
			}
			if v, ok := f["css"]; ok {
				in.CSS = v
			}
			if find := f["find"]; find != "" {
				if n := strings.Count(in.HTML, find); n != 1 {
					return &MasterError{Status: 422, Code: "VALIDATION", Msg: fmt.Sprintf("Teks yang dicari harus ada tepat satu kali di halaman (ditemukan %d).", n), Fields: map[string]string{"find": "INVALID"}}
				}
				in.HTML = strings.Replace(in.HTML, find, f["replace"], 1)
			}
			_, _, err = svc.Save(ctx, a, in)
			return err
		},
	}
}
