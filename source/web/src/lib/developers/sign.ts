// Penanda tangan HMAC-SHA256 di browser (WebCrypto) untuk tombol "Coba" dan playground tanda tangan.
// Harus identik dengan backend `internal/extapi/signing.go` — vektor uji di bawah dikunci oleh test Go yang sama.

export const ALGORITHM = 'HMAC-SHA256';

const te = new TextEncoder();
const hex = (b: ArrayBuffer) => [...new Uint8Array(b)].map((x) => x.toString(16).padStart(2, '0')).join('');

export async function sha256Hex(text: string): Promise<string> {
  return hex(await crypto.subtle.digest('SHA-256', te.encode(text)));
}

export async function hmacHex(secret: string, message: string): Promise<string> {
  const key = await crypto.subtle.importKey('raw', te.encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  return hex(await crypto.subtle.sign('HMAC', key, te.encode(message)));
}

/** Setara Go `url.QueryEscape`: hanya A-Za-z0-9 - _ . ~ yang tidak di-escape, spasi menjadi "+". */
export function queryEscape(s: string): string {
  return encodeURIComponent(s)
    .replace(/[!'()*]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase())
    .replace(/%20/g, '+');
}

/** Parameter diurutkan menurut kunci lalu nilai, di-encode, digabung dengan "&". */
export function canonicalQuery(params: [string, string][]): string {
  const sorted = [...params].sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : a[1] < b[1] ? -1 : a[1] > b[1] ? 1 : 0));
  return sorted.map(([k, v]) => `${queryEscape(k)}=${queryEscape(v)}`).join('&');
}

export type SignInput = {
  method: string;
  path: string;
  query: [string, string][];
  timestamp: string;
  nonce: string;
  body: string;
};

export async function stringToSign(i: SignInput): Promise<string> {
  return [ALGORITHM, i.method.toUpperCase(), i.path, canonicalQuery(i.query), i.timestamp, i.nonce, await sha256Hex(i.body)].join('\n');
}

export function newNonce(): string {
  return crypto.randomUUID();
}

export type SignedHeaders = { 'X-Api-Key': string; 'X-Api-Timestamp': string; 'X-Api-Nonce': string; 'X-Api-Signature': string };

export async function signRequest(keyId: string, secret: string, i: Omit<SignInput, 'timestamp' | 'nonce'>): Promise<{ headers: SignedHeaders; stringToSign: string }> {
  const timestamp = String(Math.floor(Date.now() / 1000));
  const nonce = newNonce();
  const sts = await stringToSign({ ...i, timestamp, nonce });
  return {
    stringToSign: sts,
    headers: { 'X-Api-Key': keyId, 'X-Api-Timestamp': timestamp, 'X-Api-Nonce': nonce, 'X-Api-Signature': await hmacHex(secret, sts) }
  };
}

/** Vektor uji resmi — developer mencocokkan implementasinya dengan nilai ini. */
export const VECTOR = {
  secret: 'sk_test_0123456789abcdef',
  method: 'GET',
  path: '/ext/v1/accounting/journals',
  query: [
    ['to', '2026-10-31'],
    ['from', '2026-10-01'],
    ['outlet_id', 'a b']
  ] as [string, string][],
  timestamp: '1760000000',
  nonce: 'nonce-0123456789abcdef',
  body: '',
  signature: '4bc13ded8954dee32457b668cb562c1939966f9196610b8034fbb1e7642a1366'
};
