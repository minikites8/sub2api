<template>
  <main class="auth-portal">
    <div class="auth-portal-tools">
      <router-link to="/home" class="auth-portal-tool" :title="t('home.getStarted')">
        <Icon name="home" size="sm" />
      </router-link>
      <LocaleSwitcher class="auth-portal-locale" />
    </div>

    <div class="auth-portal-main">
      <header class="auth-portal-header">
        <router-link to="/home" class="auth-portal-brand">{{ siteName }}</router-link>
        <p class="auth-portal-status">
          <span aria-hidden="true"></span>
          SYSTEM STATUS: OPERATIONAL
        </p>
      </header>

      <section class="auth-portal-card" :class="{ 'auth-portal-card--wide': wide }">
        <slot :is-dark="true" />
      </section>
    </div>

    <footer class="auth-portal-footer">
      <div>
        <router-link to="/home" class="auth-portal-footer-brand">{{ siteName }}</router-link>
        <p>&copy; {{ currentYear }} {{ siteName }}. AI-native model gateway.</p>
      </div>
      <nav aria-label="Footer navigation">
        <router-link to="/docs">Docs</router-link>
        <router-link to="/pricing">Pricing</router-link>
      </nav>
    </footer>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

withDefaults(
  defineProps<{
    siteName?: string
    wide?: boolean
  }>(),
  {
    siteName: 'Sub2API',
    wide: false
  }
)

const { t } = useI18n()
const currentYear = computed(() => new Date().getFullYear())
const htmlElement = document.documentElement
const hadDarkTheme = htmlElement.classList.contains('dark')

htmlElement.classList.add('dark')

onBeforeUnmount(() => {
  htmlElement.classList.toggle('dark', hadDarkTheme)
})
</script>

<style>
.auth-portal {
  --auth-bg: #071118;
  --auth-surface: #08141c;
  --auth-surface-strong: #0b1922;
  --auth-grid: rgb(103 136 126 / 8%);
  --auth-border: #1a302f;
  --auth-border-strong: #395449;
  --auth-text: #dce7e3;
  --auth-muted: #91a39b;
  --auth-faint: #51635b;
  --auth-accent: #00e38b;
  --auth-accent-hover: #13f09a;
  --auth-on-accent: #03130d;
  --auth-error: #ff8d84;
  --auth-error-bg: rgb(255 91 82 / 9%);
  --auth-success: #22e69a;
  display: flex;
  min-height: 100vh;
  flex-direction: column;
  overflow-x: hidden;
  background-color: var(--auth-bg);
  background-image:
    linear-gradient(var(--auth-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--auth-grid) 1px, transparent 1px);
  background-size: 32px 32px;
  color: var(--auth-text);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

.auth-portal-tools {
  position: absolute;
  z-index: 10;
  top: 22px;
  right: 26px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.auth-portal-tool {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border: 1px solid var(--auth-border);
  border-radius: 4px;
  background: color-mix(in srgb, var(--auth-surface) 84%, transparent);
  color: var(--auth-muted);
  transition: border-color 150ms ease, color 150ms ease, background-color 150ms ease;
}

.auth-portal-tool:hover,
.auth-portal-tool:focus-visible {
  border-color: var(--auth-accent);
  color: var(--auth-accent);
  outline: none;
}

.auth-portal-locale > div > button,
.auth-portal-locale > button {
  min-height: 38px;
  border: 1px solid var(--auth-border);
  border-radius: 4px !important;
  background: color-mix(in srgb, var(--auth-surface) 84%, transparent);
  color: var(--auth-muted) !important;
}

.auth-portal-main {
  display: flex;
  width: min(100% - 32px, 1180px);
  flex: 1;
  flex-direction: column;
  align-items: center;
  margin: 0 auto;
  padding: 74px 0 80px;
}

.auth-portal-header {
  width: 100%;
  text-align: center;
}

.auth-portal-brand {
  display: inline-block;
  max-width: min(900px, 100%);
  overflow-wrap: anywhere;
  color: var(--auth-accent);
  font-size: clamp(2.25rem, 6vw, 4rem);
  font-weight: 900;
  letter-spacing: 0;
  line-height: 1;
  text-transform: uppercase;
}

.auth-portal-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-top: 18px;
  color: var(--auth-text);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.75rem;
  letter-spacing: 0.12em;
}

.auth-portal-status span {
  width: 10px;
  height: 10px;
  flex: 0 0 10px;
  border-radius: 50%;
  background: var(--auth-accent);
  box-shadow: 0 0 14px rgb(0 227 139 / 55%);
}

.auth-portal-card {
  width: min(100%, 560px);
  margin-top: 54px;
  border: 1px solid var(--auth-border);
  border-radius: 2px;
  background: color-mix(in srgb, var(--auth-surface) 96%, transparent);
  padding: 52px 60px 48px;
}

.auth-portal-card--wide {
  width: min(100%, 640px);
}

.auth-portal-heading h1 {
  color: var(--auth-text);
  font-size: 1.75rem;
  font-weight: 800;
  line-height: 1.2;
}

.auth-portal-heading p {
  margin-top: 12px;
  color: var(--auth-muted);
  font-size: 0.975rem;
  line-height: 1.65;
}

.auth-portal-form {
  display: grid;
  gap: 24px;
  margin-top: 34px;
}

.auth-portal-field {
  display: grid;
  gap: 8px;
}

.auth-portal-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.auth-portal-field label,
.auth-portal-label-row label {
  color: var(--auth-text);
  font-size: 0.8rem;
  font-weight: 700;
}

.auth-portal-optional {
  margin-left: 5px;
  color: var(--auth-faint);
  font-size: 0.7rem;
  font-weight: 500;
}

.auth-portal-text-link,
.auth-portal-switch a {
  color: var(--auth-accent);
  font-size: 0.8rem;
  font-weight: 800;
  text-decoration: none;
}

.auth-portal-text-link:hover,
.auth-portal-switch a:hover {
  text-decoration: underline;
  text-underline-offset: 4px;
}

.auth-portal-input {
  display: flex;
  min-height: 56px;
  align-items: center;
  gap: 14px;
  border-bottom: 1px solid var(--auth-border-strong);
  color: var(--auth-muted);
  transition: border-color 150ms ease, color 150ms ease;
}

.auth-portal-input:focus-within {
  border-color: var(--auth-accent);
  color: var(--auth-accent);
}

.auth-portal-field--error .auth-portal-input {
  border-color: var(--auth-error);
}

.auth-portal-input > svg {
  flex: 0 0 auto;
}

.auth-portal-input input {
  min-width: 0;
  flex: 1;
  border: 0;
  background: transparent;
  color: var(--auth-text);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.9rem;
  outline: none;
}

.auth-portal-input input::placeholder {
  color: var(--auth-faint);
}

.auth-portal-input input:disabled {
  cursor: wait;
  opacity: 0.6;
}

.auth-portal-input-action,
.auth-portal-validation-icon {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  place-items: center;
  color: var(--auth-muted);
}

.auth-portal-input-action:hover,
.auth-portal-input-action:focus-visible {
  color: var(--auth-accent);
  outline: none;
}

.auth-portal-hint,
.auth-portal-field-message {
  color: var(--auth-faint);
  font-size: 0.75rem;
  line-height: 1.5;
}

.auth-portal-field-message--error {
  color: var(--auth-error);
}

.auth-portal-field-message--success {
  color: var(--auth-success);
}

.auth-portal-alert {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  border: 1px solid color-mix(in srgb, var(--auth-error) 45%, transparent);
  border-radius: 4px;
  background: var(--auth-error-bg);
  padding: 11px 12px;
  color: var(--auth-error);
  font-size: 0.8rem;
  line-height: 1.5;
}

.auth-portal-alert--warning {
  border-color: #9b7b36;
  background: rgb(203 153 45 / 8%);
  color: #e1bd68;
}

.auth-portal-turnstile {
  overflow: hidden;
}

.auth-portal-primary {
  display: inline-flex;
  min-height: 58px;
  width: 100%;
  align-items: center;
  justify-content: center;
  gap: 12px;
  border-radius: 2px;
  background: var(--auth-accent);
  color: var(--auth-on-accent);
  font-size: 0.95rem;
  font-weight: 900;
  transition: background-color 150ms ease, transform 150ms ease, opacity 150ms ease;
}

.auth-portal-primary:hover:enabled {
  background: var(--auth-accent-hover);
}

.auth-portal-primary:active:enabled {
  transform: translateY(1px);
}

.auth-portal-primary:disabled {
  cursor: wait;
  opacity: 0.48;
}

.auth-portal-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: auth-portal-spin 700ms linear infinite;
}

.auth-portal-agreement:empty {
  display: none;
}

.auth-portal-oauth {
  display: grid;
  gap: 12px;
}

.auth-portal-divider {
  display: flex;
  align-items: center;
  gap: 16px;
  margin: 4px 0 12px;
  color: var(--auth-faint);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.7rem;
  text-transform: uppercase;
}

.auth-portal-divider::before,
.auth-portal-divider::after {
  height: 1px;
  flex: 1;
  background: var(--auth-border);
  content: '';
}

.auth-portal-oauth .btn.btn-secondary {
  min-height: 56px;
  width: 100%;
  justify-content: center;
  border: 1px solid var(--auth-border);
  border-radius: 2px;
  background: transparent;
  color: var(--auth-text);
  box-shadow: none;
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.8rem;
  font-weight: 600;
  transition: border-color 150ms ease, color 150ms ease, background-color 150ms ease;
}

.auth-portal-oauth .btn.btn-secondary:hover:enabled {
  border-color: var(--auth-accent);
  background: color-mix(in srgb, var(--auth-accent) 5%, transparent);
  color: var(--auth-accent);
}

.auth-portal-oauth .btn.btn-secondary:disabled {
  cursor: wait;
  opacity: 0.5;
}

.auth-portal-oauth .space-y-4,
.auth-portal-oauth .space-y-3 {
  display: grid;
  gap: 12px;
}

.auth-portal-oauth .space-y-4 > :not([hidden]) ~ :not([hidden]),
.auth-portal-oauth .space-y-3 > :not([hidden]) ~ :not([hidden]) {
  margin-top: 0;
}

.auth-portal-oauth .grid {
  gap: 12px;
}

.auth-portal-oauth [data-testid='wechat-oauth-hint'] {
  color: var(--auth-muted);
  font-size: 0.75rem;
}

.auth-portal-switch {
  margin-top: 38px;
  border-top: 1px solid var(--auth-border);
  padding-top: 28px;
  color: var(--auth-muted);
  text-align: center;
  font-size: 0.9rem;
}

.auth-portal-footer {
  display: flex;
  min-height: 132px;
  align-items: center;
  justify-content: space-between;
  gap: 32px;
  border-top: 1px solid var(--auth-border-strong);
  background: color-mix(in srgb, var(--auth-bg) 96%, transparent);
  padding: 28px 32px;
}

.auth-portal-footer-brand {
  color: var(--auth-accent);
  font-size: 1.45rem;
  font-weight: 900;
}

.auth-portal-footer p {
  margin-top: 6px;
  color: var(--auth-muted);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.72rem;
}

.auth-portal-footer nav {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 28px;
}

.auth-portal-footer nav a {
  color: var(--auth-muted);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.75rem;
  font-weight: 700;
}

.auth-portal-footer nav a:hover {
  color: var(--auth-accent);
}

@keyframes auth-portal-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 720px) {
  .auth-portal-tools {
    position: static;
    justify-content: flex-end;
    padding: 12px 12px 0;
  }

  .auth-portal-main {
    width: min(100% - 24px, 560px);
    padding: 32px 0 56px;
  }

  .auth-portal-brand {
    font-size: clamp(2rem, 12vw, 3.25rem);
  }

  .auth-portal-status {
    font-size: 0.64rem;
  }

  .auth-portal-card {
    margin-top: 40px;
    padding: 36px 24px 34px;
  }

  .auth-portal-footer {
    align-items: flex-start;
    flex-direction: column;
    padding: 28px 20px;
  }

  .auth-portal-footer nav {
    justify-content: flex-start;
    gap: 20px;
  }
}

@media (max-width: 420px) {
  .auth-portal-main {
    width: calc(100% - 16px);
  }

  .auth-portal-card {
    padding-inline: 18px;
  }

  .auth-portal-label-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 7px;
  }
}
</style>
