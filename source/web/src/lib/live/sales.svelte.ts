// Penjualan langsung hari ini: SSE hanya membawa SINYAL "ada perubahan"; ringkasan diambil lewat GET biasa
// (disatukan, maks sekali per ~1 detik). Aliran ditutup saat tab disembunyikan dan dibuka lagi saat terlihat.
import { api, apiStream } from '#lib/api/client.ts';

export type LiveTotals = { count: number; total: string };
export type LiveSummary = {
  date: string;
  timezone: string;
  as_of: string;
  sales: LiveTotals;
  average: string;
  returns: LiveTotals;
  net: string;
  yesterday: LiveTotals;
  hours: { hour: number; count: number; total: string }[];
  recent: { id: string; doc_no: string; created_at: string; cashier: string; member?: string; line_count: number; total: string }[];
};
export type LiveStatus = 'connecting' | 'live' | 'offline';

const WATCHDOG_MS = 45_000; // server mengirim ping tiap 20 dtk
const REFRESH_GAP_MS = 800;
const FALLBACK_MS = 5 * 60_000; // jaring pengaman (mis. pergantian hari, sinyal terlewat)

export class LiveSales {
  data = $state<LiveSummary | null>(null);
  status = $state<LiveStatus>('connecting');
  failed = $state(false); // ringkasan gagal dimuat (data lama tetap ditampilkan)

  #running = false;
  #gen = 0;
  #ctl: AbortController | null = null;
  #refreshTimer: ReturnType<typeof setTimeout> | undefined;
  #fallback: ReturnType<typeof setInterval> | undefined;
  #lastRefresh = 0;
  #onVisible = () => (document.hidden ? this.#pause() : this.#resume());

  start() {
    if (this.#running) return;
    this.#running = true;
    document.addEventListener('visibilitychange', this.#onVisible);
    if (!document.hidden) this.#resume();
  }

  stop() {
    this.#running = false;
    document.removeEventListener('visibilitychange', this.#onVisible);
    this.#pause();
  }

  /** Muat ulang ringkasan sekarang (tombol Muat Ulang). */
  reload() {
    void this.#refresh();
  }

  #resume() {
    this.#pause();
    const gen = ++this.#gen;
    this.#ctl = new AbortController();
    this.status = 'connecting';
    void this.#refresh();
    void this.#loop(gen, this.#ctl);
    this.#fallback = setInterval(() => void this.#refresh(), FALLBACK_MS);
  }

  #pause() {
    this.#gen++;
    this.#ctl?.abort();
    this.#ctl = null;
    clearTimeout(this.#refreshTimer);
    clearInterval(this.#fallback);
    this.#fallback = undefined;
  }

  async #refresh() {
    const gen = this.#gen;
    this.#lastRefresh = Date.now();
    try {
      const res = await api<LiveSummary>('/live/sales/today');
      if (gen !== this.#gen) return; // outlet/tab berganti selama menunggu
      this.data = res;
      this.failed = false;
    } catch {
      if (gen === this.#gen) this.failed = true;
    }
  }

  #schedule() {
    if (this.#refreshTimer) return; // sudah ada yang menunggu: sinyal beruntun menyatu
    const wait = Math.max(0, REFRESH_GAP_MS - (Date.now() - this.#lastRefresh));
    this.#refreshTimer = setTimeout(() => {
      this.#refreshTimer = undefined;
      void this.#refresh();
    }, wait);
  }

  async #loop(gen: number, ctl: AbortController) {
    let delay = 1000;
    while (gen === this.#gen) {
      let dog: ReturnType<typeof setTimeout> | undefined;
      const feed = () => {
        clearTimeout(dog);
        dog = setTimeout(() => ctl.abort(), WATCHDOG_MS);
      };
      try {
        feed();
        const res = await apiStream('/live/sales/stream', ctl.signal);
        const reader = res.body!.pipeThrough(new TextDecoderStream()).getReader();
        let buf = '';
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          feed();
          buf += value;
          let i: number;
          while ((i = buf.indexOf('\n\n')) >= 0) {
            const block = buf.slice(0, i);
            buf = buf.slice(i + 2);
            if (block.includes('event: ready')) {
              if (gen === this.#gen) this.status = 'live';
              delay = 1000;
              this.#schedule(); // sinyal yang terlewat sebelum tersambung
            } else if (block.includes('event: sales')) {
              this.#schedule();
            }
          }
        }
        delay = 1000; // ditutup server (batas umur aliran): sambung lagi segera
      } catch {
        if (gen !== this.#gen) return;
        this.status = 'offline';
        await new Promise((r) => setTimeout(r, delay));
        delay = Math.min(delay * 2, 30_000);
      } finally {
        clearTimeout(dog);
      }
      if (gen === this.#gen) {
        // Watchdog memutus aliran → ctl lama sudah aborted; buat baru agar fetch berikutnya tidak langsung batal.
        if (ctl.signal.aborted) {
          ctl = new AbortController();
          this.#ctl = ctl;
        }
      }
    }
  }
}
