// Pembuat contoh kode (5 bahasa) untuk request bertanda tangan. Tiap contoh memuat rumus tanda tangan lengkap
// agar bisa disalin apa adanya; secret dibaca dari variabel lingkungan, bukan ditulis di kode.
import { canonicalQuery } from './sign.ts';

export type Lang = 'curl' | 'javascript' | 'python' | 'php' | 'go';
export const LANGS: { id: Lang; label: string; ext: string }[] = [
  { id: 'curl', label: 'cURL', ext: 'sh' },
  { id: 'javascript', label: 'Node.js', ext: 'js' },
  { id: 'python', label: 'Python', ext: 'py' },
  { id: 'php', label: 'PHP', ext: 'php' },
  { id: 'go', label: 'Go', ext: 'go' }
];

export type SnippetInput = {
  base: string;
  method: string;
  path: string; // sudah memuat prefix /ext/v1
  query: [string, string][];
  body: string;
};

const q = (s: string) => JSON.stringify(s);
const sq = (s: string) => `'${s.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;

export function snippet(lang: Lang, i: SnippetInput): string {
  const method = i.method.toUpperCase();
  const cq = canonicalQuery(i.query);
  const url = i.base + i.path + (cq ? `?${cq}` : '');
  const hasBody = i.body.trim() !== '';
  switch (lang) {
    case 'curl':
      return `#!/usr/bin/env bash
# Butuh: curl, openssl, uuidgen. Isi KEY_ID dan SECRET dari kredensial Anda.
KEY_ID="\${API_KEY_ID:?}"; SECRET="\${API_SECRET:?}"
METHOD=${q(method)}
URL_PATH=${q(i.path)}
QUERY=${q(cq)}               # parameter diurutkan menurut nama
BODY=${hasBody ? `'${i.body.replace(/'/g, `'\\''`)}'` : "''"}

TS=$(date +%s)
NONCE=$(uuidgen | tr 'A-Z' 'a-z')
BODY_HASH=$(printf '%s' "$BODY" | openssl dgst -sha256 -hex | sed 's/^.* //')
STRING_TO_SIGN=$(printf 'HMAC-SHA256\\n%s\\n%s\\n%s\\n%s\\n%s\\n%s' "$METHOD" "$URL_PATH" "$QUERY" "$TS" "$NONCE" "$BODY_HASH")
SIGNATURE=$(printf '%s' "$STRING_TO_SIGN" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.* //')

curl -sS -X "$METHOD" "${i.base}$URL_PATH\${QUERY:+?$QUERY}" \\
  -H "X-Api-Key: $KEY_ID" \\
  -H "X-Api-Timestamp: $TS" \\
  -H "X-Api-Nonce: $NONCE" \\
  -H "X-Api-Signature: $SIGNATURE" \\
  -H "Accept: application/json"${hasBody ? ` \\
  -H "Content-Type: application/json" \\
  --data-binary "$BODY"` : ''}
`;
    case 'javascript':
      return `// Node.js 18+ (tanpa dependensi)
import crypto from 'node:crypto';

const esc = (s) => encodeURIComponent(s).replace(/[!'()*]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase()).replace(/%20/g, '+');
const canonicalQuery = (pairs) => [...pairs].sort((a, b) => (a[0] + '\\0' + a[1] < b[0] + '\\0' + b[1] ? -1 : 1))
  .map(([k, v]) => esc(k) + '=' + esc(v)).join('&');

async function call(method, path, query = [], body = '') {
  const ts = String(Math.floor(Date.now() / 1000));
  const nonce = crypto.randomUUID();
  const cq = canonicalQuery(query);
  const bodyHash = crypto.createHash('sha256').update(body).digest('hex');
  const stringToSign = ['HMAC-SHA256', method, path, cq, ts, nonce, bodyHash].join('\\n');
  const signature = crypto.createHmac('sha256', process.env.API_SECRET).update(stringToSign).digest('hex');
  const res = await fetch(${q(i.base)} + path + (cq ? '?' + cq : ''), {
    method,
    headers: {
      'X-Api-Key': process.env.API_KEY_ID, 'X-Api-Timestamp': ts, 'X-Api-Nonce': nonce, 'X-Api-Signature': signature,
      'Content-Type': 'application/json', Accept: 'application/json'
    },
    body: body || undefined
  });
  return { status: res.status, data: await res.json().catch(() => null) };
}

console.log(await call(${q(method)}, ${q(i.path)}, ${JSON.stringify(i.query)}, ${hasBody ? `${q(i.body)}` : "''"}));
`;
    case 'python':
      return `# Python 3.8+  ·  pip install requests
import hashlib, hmac, os, time, uuid
from urllib.parse import quote_plus
import requests

def canonical_query(pairs):
    return "&".join(f"{quote_plus(k, safe='-_.~')}={quote_plus(v, safe='-_.~')}" for k, v in sorted(pairs))

def call(method, path, query=(), body=""):
    ts, nonce = str(int(time.time())), str(uuid.uuid4())
    cq = canonical_query(query)
    body_hash = hashlib.sha256(body.encode()).hexdigest()
    string_to_sign = "\\n".join(["HMAC-SHA256", method, path, cq, ts, nonce, body_hash])
    signature = hmac.new(os.environ["API_SECRET"].encode(), string_to_sign.encode(), hashlib.sha256).hexdigest()
    r = requests.request(
        method, ${q(i.base)} + path + (("?" + cq) if cq else ""),
        headers={"X-Api-Key": os.environ["API_KEY_ID"], "X-Api-Timestamp": ts, "X-Api-Nonce": nonce,
                 "X-Api-Signature": signature, "Content-Type": "application/json", "Accept": "application/json"},
        data=body.encode() if body else None, timeout=30,
    )
    return r.status_code, r.json()

print(call(${q(method)}, ${q(i.path)}, ${JSON.stringify(i.query)}, ${hasBody ? q(i.body) : '""'}))
`;
    case 'php':
      return `<?php
// PHP 8+ (ext-curl)
function canonicalQuery(array $pairs): string {
    usort($pairs, fn($a, $b) => [$a[0], $a[1]] <=> [$b[0], $b[1]]);
    $esc = fn($s) => str_replace('%20', '+', rawurlencode($s));
    return implode('&', array_map(fn($p) => $esc($p[0]) . '=' . $esc($p[1]), $pairs));
}

function call(string $method, string $path, array $query = [], string $body = ''): array {
    $ts = (string) time();
    $nonce = bin2hex(random_bytes(16));
    $cq = canonicalQuery($query);
    $stringToSign = implode("\\n", ['HMAC-SHA256', $method, $path, $cq, $ts, $nonce, hash('sha256', $body)]);
    $signature = hash_hmac('sha256', $stringToSign, getenv('API_SECRET'));
    $ch = curl_init(${sq(i.base)} . $path . ($cq ? '?' . $cq : ''));
    curl_setopt_array($ch, [
        CURLOPT_CUSTOMREQUEST => $method, CURLOPT_RETURNTRANSFER => true, CURLOPT_TIMEOUT => 30,
        CURLOPT_HTTPHEADER => [
            'X-Api-Key: ' . getenv('API_KEY_ID'), 'X-Api-Timestamp: ' . $ts, 'X-Api-Nonce: ' . $nonce,
            'X-Api-Signature: ' . $signature, 'Content-Type: application/json', 'Accept: application/json',
        ],
        CURLOPT_POSTFIELDS => $body !== '' ? $body : null,
    ]);
    $out = curl_exec($ch);
    $status = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);
    return [$status, json_decode($out, true)];
}

print_r(call(${sq(method)}, ${sq(i.path)}, ${phpPairs(i.query)}, ${hasBody ? sq(i.body) : "''"}));
`;
    case 'go':
      return `package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func call(method, path string, query url.Values, body string) (int, string, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid()) // wajib unik per request (16–64 karakter)
	cq := canonicalQuery(query)
	sum := sha256.Sum256([]byte(body))
	stringToSign := strings.Join([]string{"HMAC-SHA256", method, path, cq, ts, nonce, hex.EncodeToString(sum[:])}, "\\n")
	mac := hmac.New(sha256.New, []byte(os.Getenv("API_SECRET")))
	mac.Write([]byte(stringToSign))

	target := ${q(i.base)} + path
	if cq != "" {
		target += "?" + cq
	}
	req, _ := http.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Api-Key", os.Getenv("API_KEY_ID"))
	req.Header.Set("X-Api-Timestamp", ts)
	req.Header.Set("X-Api-Nonce", nonce)
	req.Header.Set("X-Api-Signature", hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out), nil
}

func main() {
	status, out, err := call(${q(method)}, ${q(i.path)}, ${goValues(i.query)}, ${hasBody ? '`' + i.body.replace(/`/g, '` + "`" + `') + '`' : '""'})
	fmt.Println(status, out, err)
}
`;
  }
}

function phpPairs(p: [string, string][]): string {
  return '[' + p.map(([k, v]) => `[${sq(k)}, ${sq(v)}]`).join(', ') + ']';
}

function goValues(p: [string, string][]): string {
  if (p.length === 0) return 'nil';
  return 'url.Values{' + p.map(([k, v]) => `${q(k)}: {${q(v)}}`).join(', ') + '}';
}
