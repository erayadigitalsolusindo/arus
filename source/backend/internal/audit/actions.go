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
