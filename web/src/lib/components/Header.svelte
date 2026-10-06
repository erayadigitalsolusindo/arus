<script lang="ts">
  let { onmenu }: { onmenu: () => void } = $props();

  type Theme = 'light' | 'dark';
  let theme = $state<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');

  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem('theme', theme);
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }
</script>

<header class="app-header">
  <div class="absolute inset-x-0 bottom-0 h-px pointer-events-none [background:linear-gradient(90deg,transparent,var(--color-primary-500)_20%,var(--color-accent-500)_50%,var(--color-primary-500)_80%,transparent)] opacity-[0.55]"></div>

  <div class="flex items-center gap-3 px-4 lg:px-6 h-16">
    <button type="button" class="header-icon-btn lg:hidden" aria-label="Buka menu" onclick={onmenu}>
      <i class="icon-menu text-[18px]"></i>
    </button>

    <div class="hidden md:flex items-center gap-2.5 w-full max-w-[280px] h-10 pe-3.5 ps-1.5 rounded-full bg-[var(--surface-sunken)] border border-[var(--border-subtle)]">
      <span class="grid place-items-center size-7 rounded-full shrink-0 bg-[color-mix(in_oklab,var(--color-primary-500)_16%,transparent)] text-[var(--color-primary-600)]">
        <i class="icon-search text-[13px]"></i>
      </span>
      <span class="text-[13px] flex-1 text-start truncate font-medium text-[var(--text-tertiary)]">Cari barang, nota, pelanggan…</span>
    </div>

    <div class="ms-auto flex items-center gap-1">
      <button
        type="button"
        class="header-icon-btn"
        aria-label={theme === 'dark' ? 'Mode terang' : 'Mode gelap'}
        onclick={toggleTheme}
      >
        <i class={theme === 'dark' ? 'icon-sun text-[17px]' : 'icon-moon text-[17px]'}></i>
      </button>
    </div>
  </div>
</header>
