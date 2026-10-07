<script lang="ts">
  import { onMount } from 'svelte';
  import { API_URL } from '#lib/api/client.ts';
  import { t } from '#lib/i18n/index.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';

  type Status = 'checking' | 'online' | 'degraded' | 'offline';
  let status = $state<Status>('checking');
  let theme = $state<'light' | 'dark'>('light');
  const dot: Record<Status, string> = {
    checking: 'bg-[var(--color-neutral-400,#9ca3af)]',
    online: 'bg-[var(--color-success-500,#22c55e)]',
    degraded: 'bg-[var(--color-warning-500,#f59e0b)]',
    offline: 'bg-[var(--color-danger-500,#ef4444)]'
  };

  async function check() {
    try {
      const res = await fetch(`${API_URL}/healthz`, { signal: AbortSignal.timeout(4000) });
      status = res.ok ? 'online' : 'degraded';
    } catch {
      status = 'offline';
    }
  }

  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem('theme', theme);
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }

  onMount(() => {
    theme = document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
    check();
    const timer = setInterval(check, 30000);
    return () => clearInterval(timer);
  });
</script>

<footer class="relative flex flex-wrap items-center justify-between gap-x-4 gap-y-2 pt-6 text-[11.5px] text-tertiary">
  <span>© {new Date().getFullYear()} ARUS</span>

  <div class="flex items-center gap-4">
    <span class="inline-flex items-center gap-1.5" role="status" aria-live="polite">
      <span class="size-1.5 rounded-full {dot[status]} {status === 'online' ? 'status-pulse' : ''}"></span>{t(`shell.server.${status}`)}
    </span>
    <LanguageSwitcher placement="up" />
    <button type="button" class="inline-flex items-center gap-1 hover:text-[var(--color-primary-600)]" onclick={toggleTheme} aria-label={theme === 'dark' ? t('common.theme.light') : t('common.theme.dark')}>
      <i class={theme === 'dark' ? 'icon-sun-medium text-[13px]' : 'icon-moon text-[13px]'}></i>
    </button>
    <a href="#bantuan" class="inline-flex items-center gap-1 font-semibold hover:text-[var(--color-primary-600)]">
      <i class="icon-life-buoy text-[13px]"></i>{t('common.help')}
    </a>
  </div>
</footer>

<style>
  .status-pulse {
    animation: pulse 2s ease-in-out infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      box-shadow: 0 0 0 0 rgb(34 197 94 / 0.5);
    }
    50% {
      box-shadow: 0 0 0 4px rgb(34 197 94 / 0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .status-pulse {
      animation: none;
    }
  }
</style>
