-- name: MfaGet :one
SELECT email, password_hash, totp_secret_enc, totp_enabled_at, totp_last_step
FROM users WHERE tenant_id = @tenant_id AND id = @id FOR UPDATE;

-- name: MfaBegin :execrows
UPDATE users SET totp_secret_enc = @secret_enc, totp_last_step = 0
WHERE tenant_id = @tenant_id AND id = @id AND totp_enabled_at IS NULL;

-- name: MfaEnable :execrows
UPDATE users SET totp_enabled_at = now(), totp_last_step = @step
WHERE tenant_id = @tenant_id AND id = @id AND totp_secret_enc IS NOT NULL AND totp_enabled_at IS NULL;

-- name: MfaUseStep :execrows
UPDATE users SET totp_last_step = @step
WHERE tenant_id = @tenant_id AND id = @id AND totp_enabled_at IS NOT NULL AND totp_last_step < @step;

-- name: MfaReset :execrows
UPDATE users SET totp_secret_enc = NULL, totp_enabled_at = NULL, totp_last_step = 0,
       tokens_valid_after = CASE WHEN @revoke::boolean THEN @valid_after::timestamptz ELSE tokens_valid_after END
WHERE tenant_id = @tenant_id AND id = @id;

-- name: MfaRecoveryDelete :exec
DELETE FROM user_recovery_codes WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: MfaRecoveryInsert :exec
INSERT INTO user_recovery_codes (tenant_id, user_id, code_hash)
SELECT @tenant_id, @user_id, unnest(@hashes::text[]);

-- name: MfaRecoveryUse :execrows
UPDATE user_recovery_codes SET used_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND code_hash = @code_hash AND used_at IS NULL;

-- name: MfaRecoveryRemaining :one
SELECT count(*)::int FROM user_recovery_codes WHERE tenant_id = @tenant_id AND user_id = @user_id AND used_at IS NULL;

-- name: MfaEnabled :one
SELECT (totp_enabled_at IS NOT NULL)::boolean FROM users WHERE tenant_id = @tenant_id AND id = @id;
