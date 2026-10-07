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
)

// Master pendukung katalog (Fase 3.1). Aksi `<entitas>.active` = arsip/aktifkan kembali.
const (
	EntityUnit      = "unit"
	EntityCategory  = "category"
	EntityBrand     = "brand"
	EntityPrincipal = "principal"
	EntitySupplier  = "supplier"

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
