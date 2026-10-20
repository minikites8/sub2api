<template>
  <button
    type="button"
    class="captcha-trigger"
    :class="`captcha-trigger--${state}`"
    :disabled="state === 'verified'"
    :aria-busy="state === 'verifying'"
  >
    <span class="captcha-trigger-indicator" aria-hidden="true">
      <svg v-if="state === 'verifying'" class="captcha-trigger-spinner" viewBox="0 0 24 24" fill="none">
        <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2" opacity="0.2" />
        <path d="M12 3a9 9 0 0 1 9 9" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
      </svg>
      <svg v-else-if="state === 'verified'" viewBox="0 0 24 24" fill="none">
        <path d="m5 12 4 4L19 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      <svg v-else-if="state === 'error'" viewBox="0 0 24 24" fill="none">
        <path d="M12 7v6m0 4h.01" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
      </svg>
    </span>
    <span class="captcha-trigger-label" aria-live="polite">{{ buttonText }}</span>
    <svg class="captcha-trigger-shield" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="m12 3 8 3v5c0 5-4 8-8 10-4-2-8-5-8-10V6l8-3Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
      <path d="m8.5 12 2.5 2.5 4.5-5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ state: 'idle' | 'verifying' | 'verified' | 'error' }>()
const { t } = useI18n()
const buttonText = computed(() => {
  switch (props.state) {
    case 'verified': return t('auth.captchaVerified')
    case 'verifying': return t('auth.captchaVerifying')
    case 'error': return t('auth.turnstileFailed')
    default: return t('auth.captchaClickToVerify')
  }
})
</script>

<style scoped>
.captcha-trigger {
  display: flex;
  width: 100%;
  min-height: 52px;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--md-outline-variant, #d1d5db);
  border-radius: 8px;
  background: var(--md-surface, #f9fafb);
  padding: 12px 14px;
  color: var(--md-on-surface, #374151);
  text-align: left;
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1.5;
  cursor: pointer;
  transition: border-color 160ms ease, background-color 160ms ease;
}

.captcha-trigger:hover:not(:disabled) {
  border-color: var(--md-outline, #9ca3af);
  background: var(--md-surface-container, #f3f4f6);
}

.captcha-trigger:focus-visible {
  outline: 2px solid var(--md-primary, #2563eb);
  outline-offset: 3px;
}

.captcha-trigger-indicator {
  display: grid;
  width: 22px;
  height: 22px;
  flex: 0 0 22px;
  place-items: center;
  border: 1.5px solid var(--md-outline, #9ca3af);
  border-radius: 5px;
  background: var(--md-surface, #fff);
}

.captcha-trigger-indicator svg { width: 18px; height: 18px; }
.captcha-trigger-label { flex: 1; }
.captcha-trigger-shield { width: 24px; height: 24px; flex-shrink: 0; opacity: 0.55; }
.captcha-trigger--verifying { cursor: progress; }
.captcha-trigger--verifying .captcha-trigger-indicator { border-color: transparent; background: transparent; }
.captcha-trigger-spinner { animation: captcha-spin 800ms linear infinite; }

.captcha-trigger--verified,
.captcha-trigger--verified:disabled {
  border-color: #86c8a0;
  background: #f0fdf4;
  color: #15803d;
  cursor: default;
  opacity: 1;
}
.captcha-trigger--verified .captcha-trigger-indicator { border-color: currentColor; background: transparent; }
.captcha-trigger--error .captcha-trigger-indicator { border-color: currentColor; }
.captcha-trigger--error { color: var(--md-error, #b91c1c); }

:global(.dark) .captcha-trigger {
  border-color: var(--md-outline-variant, #4b5563);
  background: var(--md-surface, #1f2937);
  color: var(--md-on-surface, #e5e7eb);
}
:global(.dark) .captcha-trigger:hover:not(:disabled) { background: var(--md-surface-container, #374151); }
:global(.dark) .captcha-trigger-indicator { background: transparent; }
:global(.dark) .captcha-trigger--verified { border-color: #238653; background: #14532d4d; color: #86efac; }
:global(.dark) .captcha-trigger--error { color: var(--md-error, #fca5a5); }

@keyframes captcha-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) {
  .captcha-trigger { transition: none; }
  .captcha-trigger-spinner { animation: none; }
}
</style>
