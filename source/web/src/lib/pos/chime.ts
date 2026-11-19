// Nada pendek "pembayaran berhasil" (WebAudio, tanpa berkas suara). Dipanggil untuk semua metode bayar.
export function payChime() {
  try {
    const ctx = new AudioContext();
    [880, 1318.5].forEach((f, i) => {
      const o = ctx.createOscillator();
      const g = ctx.createGain();
      const t0 = ctx.currentTime + i * 0.14;
      o.frequency.value = f;
      g.gain.setValueAtTime(0.0001, t0);
      g.gain.exponentialRampToValueAtTime(0.25, t0 + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + 0.35);
      o.connect(g).connect(ctx.destination);
      o.start(t0);
      o.stop(t0 + 0.4);
    });
    setTimeout(() => void ctx.close(), 1000);
  } catch {
    /* tanpa suara bila peramban menolak */
  }
}

const S = ['', 'satu', 'dua', 'tiga', 'empat', 'lima', 'enam', 'tujuh', 'delapan', 'sembilan', 'sepuluh', 'sebelas'];

/** Angka bulat >= 0 menjadi kata Indonesia, mis. 125000 -> "seratus dua puluh lima ribu" (sama dengan dashboard EraPay). */
export function terbilang(n: number): string {
  if (n === 0) return 'nol';
  const w = (x: number): string => {
    if (x < 12) return S[x];
    if (x < 20) return `${S[x - 10]} belas`;
    if (x < 100) return `${S[Math.floor(x / 10)]} puluh ${S[x % 10]}`;
    if (x < 200) return `seratus ${w(x - 100)}`;
    if (x < 1000) return `${S[Math.floor(x / 100)]} ratus ${w(x % 100)}`;
    if (x < 2000) return `seribu ${w(x - 1000)}`;
    if (x < 1e6) return `${w(Math.floor(x / 1000))} ribu ${w(x % 1000)}`;
    if (x < 1e9) return `${w(Math.floor(x / 1e6))} juta ${w(x % 1e6)}`;
    return `${w(Math.floor(x / 1e9))} miliar ${w(x % 1e9)}`;
  };
  return w(Math.floor(n)).replace(/\s+/g, ' ').trim();
}

// Metode bayar yang nominalnya diucapkan: per perangkat, bawaan semua non-tunai.
export const VOICE_KINDS = ['cash', 'transfer', 'debit', 'credit_card', 'ewallet'] as const;
const VOICE_KEY = 'arus.pos.voiceKinds';
export function loadVoiceKinds(): string[] {
  try {
    const raw = JSON.parse(localStorage.getItem(VOICE_KEY) ?? 'null');
    if (Array.isArray(raw)) return raw.map(String);
  } catch {
    /* penyimpanan diblokir: pakai bawaan */
  }
  return VOICE_KINDS.filter((k) => k !== 'cash');
}
export function saveVoiceKinds(k: string[]) {
  try {
    localStorage.setItem(VOICE_KEY, JSON.stringify(k));
  } catch {
    /* abaikan */
  }
}

/** Ucapkan "Transaksi dengan nominal ... berhasil. Terima kasih". Mengembalikan false bila suara tak tersedia. */
export function speakPaid(amount: number): boolean {
  try {
    const u = new SpeechSynthesisUtterance(`Transaksi dengan nominal ${terbilang(Math.round(amount))} rupiah berhasil. Terima kasih.`);
    u.lang = 'id-ID';
    speechSynthesis.speak(u);
    return true;
  } catch {
    return false;
  }
}
