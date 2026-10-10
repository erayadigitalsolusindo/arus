package audit

// Nama aksi dan entitas yang dicatat. Format: `<entitas>.<kata kerja>` huruf kecil (dijaga CHECK di DB).
// Modul baru menambah konstantanya di sini agar daftar kejadian audit tetap terpusat dan mudah dicari.
const (
	// Entitas
	EntityUser    = "user"
	EntityRole    = "role"
	EntityOutlet  = "outlet"
	EntitySession = "session"

	// Autentikasi
	ActionRegister       = "auth.register"
	ActionLogin          = "auth.login"
	ActionPasswordReset  = "auth.password_reset"
	ActionPasswordChange = "auth.password_change"
	ActionEmailVerified  = "auth.email_verified"
	ActionOutletSwitch   = "auth.outlet_switch"
	ActionTermsAccepted  = "auth.terms_accepted"

	// Pengguna & role
	ActionUserCreate        = "user.create"
	ActionUserUpdate        = "user.update"
	ActionUserPasswordReset = "user.password_reset"
	ActionUserEmailVerify   = "user.email_verify"
	ActionRoleCreate        = "role.create"
	ActionRoleUpdate        = "role.update"
	ActionRoleDelete        = "role.delete"

	// Outlet
	ActionOutletCreate = "outlet.create"
	ActionOutletUpdate = "outlet.update"
)

// Tindakan Platform Admin yang terlihat oleh pemilik tenant (audit tenant). Pelaku tercatat "Platform: <nama>".
const (
	EntityTenant = "tenant"

	ActionPlatformImpersonate  = "platform.impersonate"
	ActionPlatformTenantStatus = "platform.tenant_status"
	ActionPlatformEditWindow   = "platform.tenant_edit_window" // batas hari edit/batal nota (diatur operator platform)
)

// Master pendukung katalog (Fase 3.1). Aksi `<entitas>.active` = arsip/aktifkan kembali.
const (
	EntityUnit        = "unit"
	EntityCategory    = "category"
	EntityBrand       = "brand"
	EntityPrincipal   = "principal"
	EntitySupplier    = "supplier"
	EntitySalesperson = "salesperson"

	ActionUnitCreate = "unit.create"
	ActionUnitUpdate = "unit.update"
	ActionUnitActive = "unit.active"

	ActionCategoryCreate = "category.create"
	ActionCategoryUpdate = "category.update"
	ActionCategoryActive = "category.active"

	ActionBrandCreate = "brand.create"
	ActionBrandUpdate = "brand.update"
	ActionBrandActive = "brand.active"

	ActionPrincipalCreate = "principal.create"
	ActionPrincipalUpdate = "principal.update"
	ActionPrincipalActive = "principal.active"

	ActionSupplierCreate = "supplier.create"
	ActionSupplierUpdate = "supplier.update"
	ActionSupplierActive = "supplier.active"

	ActionSalespersonCreate = "salesperson.create"
	ActionSalespersonUpdate = "salesperson.update"
	ActionSalespersonActive = "salesperson.active"
)

// Daftar item (Fase 3.2). `item.price` mencatat perubahan harga jual (default/cabang) terpisah dari `item.update`.
const (
	EntityItem = "item"

	ActionItemCreate = "item.create"
	ActionItemUpdate = "item.update"
	ActionItemPrice  = "item.price"
	ActionItemActive = "item.active"
)

// Gambar item (Fase 3.2, irisan C).
const (
	ActionItemImageAdd    = "item.image_add"
	ActionItemImageMain   = "item.image_main"
	ActionItemImageDelete = "item.image_delete"
)

// Stok (Fase 4.2). `stock.opening` = saldo awal satu barang/bucket diubah; `stock.opening_lock` = tanggal mulai operasional dikunci.
const (
	EntityStock = "stock"

	ActionStockOpening     = "stock.opening"
	ActionStockOpeningLock = "stock.opening_lock"
	ActionStockConvert     = "stock.convert"

	ActionStockTransferSend    = "stock.transfer_send"
	ActionStockTransferReceive = "stock.transfer_receive"
	ActionStockTransferCancel  = "stock.transfer_cancel"

	ActionStockCountCreate   = "stock.count_create"
	ActionStockCountItems    = "stock.count_items"
	ActionStockCountComplete = "stock.count_complete"
	ActionStockCountCancel   = "stock.count_cancel"
	ActionStockCountQuick    = "stock.count_quick"
)

// Penjualan (Fase 5.1).
const (
	EntitySale = "sale"

	ActionSaleCreate     = "sale.create"
	ActionSaleEdit       = "sale.edit"       // revisi nota (nota baru yang menggantikan)
	ActionSaleSuperseded = "sale.superseded" // pada nota lama: sudah digantikan revisi
	ActionSaleVoid       = "sale.void"
	ActionSaleReprint    = "sale.reprint" // cetak ulang struk (detail: doc_no, copy)
)

const (
	EntitySaleReturn       = "sale_return"
	ActionSaleReturnCreate = "sale_return.create"
	ActionSaleReturnVoid   = "sale_return.void"
)

// Piutang member (penjualan kredit).
const (
	ActionReceivablePay         = "receivable.pay"
	ActionReceivableSettle      = "receivable.settle"       // pelunasan kolektif per member
	ActionReceivableOpening     = "receivable.opening"      // saldo awal piutang (onboarding)
	ActionReceivableOpeningVoid = "receivable.opening_void" // batal saldo awal piutang
)

// Shift kasir (buka/tutup laci).
const (
	EntityShift      = "shift"
	ActionShiftOpen  = "shift.open"
	ActionShiftClose = "shift.close"
)

// Hutang pemasok (pembelian kredit).
const (
	EntityPayable       = "payable"
	ActionPayablePay    = "payable.pay"
	ActionPayableSettle = "payable.settle" // pelunasan kolektif per pemasok
)

// Saldo titipan: deposit member (top-up / tarik) dan kredit pemasok (pencairan).
const (
	EntityMemberDeposit         = "member_deposit"
	EntitySupplierCredit        = "supplier_credit"
	ActionDepositCash           = "member_deposit.cash"
	ActionSupplierCreditCashOut = "supplier_credit.cash_out"
)

// Persetujuan (PIN) — ubah harga di kasir.
const (
	ActionSalePriceOverride = "sale.price_override"
	ActionSaleLineDiscount  = "sale.line_discount"
	ActionUserPinSet        = "user.pin_set"
)

// Member & level member (Fase 3.3). `member.points_adjust` = penyesuaian poin manual.
const (
	EntityReceivable  = "receivable"
	EntityMember      = "member"
	EntityMemberLevel = "member_level"

	ActionMemberCreate       = "member.create"
	ActionMemberUpdate       = "member.update"
	ActionMemberActive       = "member.active"
	ActionMemberCover        = "member.cover"
	ActionMemberPointsAdjust = "member.points_adjust"

	ActionMemberLevelCreate = "member_level.create"
	ActionMemberLevelUpdate = "member_level.update"
	ActionMemberLevelActive = "member_level.active"
)

// Kupon belanja global. `voucher.active` = arsip/aktifkan kembali.
const (
	EntityVoucher = "voucher"

	ActionVoucherCreate = "voucher.create"
	ActionVoucherUpdate = "voucher.update"
	ActionVoucherActive = "voucher.active"
)

// Metode pembayaran. `payment_method.active` = arsip/aktifkan kembali.
const (
	EntityPaymentMethod = "payment_method"

	ActionPaymentMethodCreate = "payment_method.create"
	ActionPaymentMethodUpdate = "payment_method.update"
	ActionPaymentMethodActive = "payment_method.active"
)

// Pembelian (Fase 6.2).
const (
	EntityPurchase = "purchase"

	ActionPurchaseCreate     = "purchase.create"
	ActionPurchaseEdit       = "purchase.edit"
	ActionPurchaseSuperseded = "purchase.superseded"
	ActionPurchaseVoid       = "purchase.void"
)

// Retur pembelian (Fase 6.6).
const (
	EntityPurchaseReturn = "purchase_return"

	ActionPurchaseReturnCreate = "purchase_return.create"
	ActionPurchaseReturnVoid   = "purchase_return.void"
)

// Akuntansi SIAK (COA, periode, jurnal).
const (
	EntityAccount          = "account"
	EntityAccountingPeriod = "accounting_period"
	EntityJournal          = "journal"

	ActionAccountCreate = "account.create"
	ActionAccountUpdate = "account.update"
	ActionAccountDelete = "account.delete"
	ActionAccountSeed   = "account.seed" // template COA retail

	ActionPeriodClose  = "accounting_period.close"
	ActionPeriodReopen = "accounting_period.reopen"

	ActionJournalSave    = "journal.save" // buat/ubah draf
	ActionJournalDelete  = "journal.delete"
	ActionJournalPost    = "journal.post"
	ActionJournalReverse = "journal.reverse"
)
