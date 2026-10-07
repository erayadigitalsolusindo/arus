-- +goose Up

INSERT INTO app_settings (key, value, description) VALUES (
    'auth.rate_limits',
    '{"register_per_ip": 10, "forgot_per_ip": 10, "forgot_per_email": 3, "reset_per_ip": 20, "verify_per_ip": 30, "resend_verification_per_user": 3}',
    'Batas permintaan endpoint auth per jam: *_per_ip = per IP klien; forgot_per_email = permintaan reset password per email (kelebihan diabaikan diam-diam); resend_verification_per_user = kirim ulang email verifikasi per pengguna. Nilai 1..100000; tidak valid = dipakai nilai sebelumnya.'
);

-- +goose Down
DELETE FROM app_settings WHERE key = 'auth.rate_limits';
