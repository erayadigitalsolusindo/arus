<script lang="ts">
  import { onMount } from 'svelte';
  import { turnstileSiteKey } from '#lib/captcha.ts';

  type TurnstileApi = { render: (el: HTMLElement, o: Record<string, unknown>) => string; reset: (id: string) => void; remove: (id: string) => void };
  let { token = $bindable('') }: { token?: string } = $props();
  let el = $state<HTMLDivElement>();
  let widgetId: string | undefined;

  let loading: Promise<void> | undefined;
  const api = () => (window as unknown as { turnstile?: TurnstileApi }).turnstile;
  function loadScript(): Promise<void> {
    loading ??= new Promise((resolve, reject) => {
      if (api()) return resolve();
      const s = document.createElement('script');
      s.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
      s.async = true;
      s.onload = () => resolve();
      s.onerror = () => reject(new Error('turnstile'));
      document.head.append(s);
    });
    return loading;
  }

  /** Token Turnstile hanya berlaku sekali: panggil setelah setiap percobaan kirim. */
  export function reset() {
    token = '';
    const t = api();
    if (widgetId && t) t.reset(widgetId);
  }

  onMount(() => {
    if (!turnstileSiteKey) return;
    void loadScript()
      .then(() => {
        const t = api();
        if (!t || !el) return;
        widgetId = t.render(el, {
          sitekey: turnstileSiteKey,
          theme: 'auto',
          callback: (v: string) => (token = v),
          'expired-callback': () => (token = ''),
          'error-callback': () => (token = '')
        });
      })
      .catch(() => (token = ''));
    return () => {
      const t = api();
      if (widgetId && t) t.remove(widgetId);
    };
  });
</script>

{#if turnstileSiteKey}<div bind:this={el} class="flex justify-center min-h-[65px]"></div>{/if}
