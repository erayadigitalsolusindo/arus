#!/usr/bin/env bash
# Cek kesehatan ARUS dalam satu layar. [OK] = sehat, [MASALAH] = perlu dilihat.
B=/www/wwwroot/arus
ok()  { echo "  [OK]       $*"; }
bad() { echo "  [MASALAH]  $*"; }

echo "== API"
h=$(curl -s --max-time 5 http://127.0.0.1:8080/healthz || true)
case "$h" in *'"status":"ok"'*) ok "healthz: $h";; *) bad "healthz: ${h:-tidak menjawab}";; esac
systemctl is-active --quiet arus-api && ok "service arus-api aktif" || bad "service arus-api tidak aktif"

echo "== Dari internet"
for u in https://arus.erayadigital.co.id/login https://arus-api.erayadigital.co.id/healthz; do
  c=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$u" || true)
  [ "$c" = 200 ] && ok "$u -> $c" || bad "$u -> $c"
done

echo "== Docker"
for n in aciraba_postgres aciraba_redis; do
  s=$(sudo docker inspect -f '{{.State.Health.Status}}' $n 2>/dev/null || echo tidak-ada)
  [ "$s" = healthy ] && ok "$n: $s" || bad "$n: $s"
done

echo "== Database"
v=$(sudo docker exec aciraba_postgres psql -U aciraba -d aciraba -tAc 'select max(version_id) from goose_db_version' 2>/dev/null || true)
[ -n "$v" ] && ok "versi migrasi terakhir: $v" || bad "tidak bisa membaca versi migrasi"

echo "== Backup"
last=$(ls -t $B/backups/*.dump 2>/dev/null | head -1)
if [ -z "$last" ]; then
  bad "belum ada berkas backup"
else
  age=$(( ( $(date +%s) - $(stat -c %Y "$last") ) / 3600 ))
  [ "$age" -le 30 ] && ok "backup terakhir $age jam lalu: $(basename "$last")" || bad "backup terakhir $age jam lalu (lebih dari 30 jam)"
fi

echo "== Disk"
p=$(df --output=pcent /www | tail -1 | tr -dc 0-9)
[ "$p" -lt 85 ] && ok "disk terpakai ${p}%" || bad "disk terpakai ${p}%"

echo "== Error API (1 jam terakhir)"
n=$(sudo journalctl -u arus-api --since '1 hour ago' -p err --no-pager -q | wc -l)
[ "$n" -eq 0 ] && ok "tidak ada error" || bad "$n baris error (lihat: sudo journalctl -u arus-api -p err -n 20)"
