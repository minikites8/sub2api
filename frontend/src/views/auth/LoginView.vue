<template>
  <main class="md3-login" :class="{ 'md3-login-dark': isDark }">
    <div class="md3-login-shell">
      <section class="md3-login-context">
        <router-link to="/home" class="md3-brand">
          <span class="md3-logo">
            <img v-if="siteLogo" :src="siteLogo" :alt="siteName" />
            <span v-else>{{ brandInitial }}</span>
          </span>
          <span class="md3-brand-copy">
            <strong>{{ siteName }}</strong>
            <span>{{ siteSubtitle }}</span>
          </span>
        </router-link>

        <div class="md3-context-copy">
          <span class="md3-chip">
            <Icon name="shield" size="sm" />
            {{ t('home.tags.stickySession') }}
          </span>
          <h1>{{ siteName }}</h1>
          <p>{{ siteSubtitle }}</p>
        </div>

        <div class="md3-context-list" aria-hidden="true">
          <div v-for="item in contextItems" :key="item.label" class="md3-context-item">
            <span>
              <Icon :name="item.icon" size="sm" />
            </span>
            <div>
              <strong>{{ item.label }}</strong>
              <small>{{ item.value }}</small>
            </div>
          </div>
        </div>

        <p class="md3-copyright">
          &copy; {{ currentYear }} {{ siteName }}.
        </p>
      </section>

      <section class="md3-login-panel" aria-labelledby="login-title">
        <header class="md3-panel-actions">
          <router-link to="/home" class="md3-icon-button" :title="t('home.getStarted')" :aria-label="t('home.getStarted')">
            <Icon name="home" size="md" />
          </router-link>
          <LocaleSwitcher class="md3-locale" />
          <button
            type="button"
            class="md3-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-pressed="isDark"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
        </header>

        <div class="md3-auth-card">
          <div class="md3-card-heading">
            <span>{{ t('home.login') }}</span>
            <h2 id="login-title">{{ t('auth.welcomeBack') }}</h2>
            <p>{{ t('auth.signInToAccount') }}</p>
          </div>

          <form @submit.prevent="handleLogin" class="md3-form">
            <div class="md3-field" :class="{ 'md3-field-error': errors.email }">
              <label for="email">{{ t('auth.emailLabel') }}</label>
              <div class="md3-input-shell">
                <Icon name="mail" size="md" class="md3-input-icon" />
                <input
                  id="email"
                  v-model="formData.email"
                  type="email"
                  required
                  autofocus
                  autocomplete="email"
                  :disabled="authActionDisabled"
                  :placeholder="t('auth.emailPlaceholder')"
                />
              </div>
            </div>

            <div class="md3-field" :class="{ 'md3-field-error': errors.password }">
              <div class="md3-label-row">
                <label for="password">{{ t('auth.passwordLabel') }}</label>
                <router-link
                  v-if="passwordResetEnabled && !backendModeEnabled"
                  to="/forgot-password"
                  class="md3-text-link"
                >
                  {{ t('auth.forgotPassword') }}
                </router-link>
              </div>
              <div class="md3-input-shell">
                <Icon name="lock" size="md" class="md3-input-icon" />
                <input
                  id="password"
                  v-model="formData.password"
                  :type="showPassword ? 'text' : 'password'"
                  required
                  autocomplete="current-password"
                  :disabled="authActionDisabled"
                  :placeholder="t('auth.passwordPlaceholder')"
                />
                <button
                  type="button"
                  class="md3-password-button"
                  :disabled="authActionDisabled"
                  :aria-label="showPassword ? 'Hide password' : 'Show password'"
                  @click="showPassword = !showPassword"
                >
                  <Icon v-if="showPassword" name="eyeOff" size="md" />
                  <Icon v-else name="eye" size="md" />
                </button>
              </div>
            </div>

            <div v-if="errorMessage" class="md3-error-message" role="alert">
              <Icon name="exclamationCircle" size="sm" />
              <span>{{ errorMessage }}</span>
            </div>

            <div v-if="turnstileEnabled && turnstileSiteKey" class="md3-turnstile">
              <TurnstileWidget
                ref="turnstileRef"
                :site-key="turnstileSiteKey"
                :theme="isDark ? 'dark' : 'light'"
                @verify="onTurnstileVerify"
                @expire="onTurnstileExpire"
                @error="onTurnstileError"
              />
            </div>

            <button
              type="submit"
              :disabled="authActionDisabled || (turnstileEnabled && !turnstileToken)"
              class="md3-primary-button"
            >
              <span v-if="isLoading" class="md3-spinner" aria-hidden="true"></span>
              <Icon v-else name="login" size="md" />
              {{ isLoading ? t('auth.signingIn') : t('auth.signIn') }}
            </button>

            <div class="md3-agreement">
              <LoginAgreementPrompt
                v-if="loginAgreementEnabled"
                :accepted="agreementAccepted"
                :documents="loginAgreementDocuments"
                :mode="loginAgreementMode"
                :updated-at="loginAgreementUpdatedAt"
                :visible="showAgreementModal"
                @accept="acceptLoginAgreement"
                @reject="rejectLoginAgreement"
                @open="showAgreementModal = true"
              />
            </div>

            <div v-if="showOAuthLogin" class="md3-oauth-stack">
              <div class="md3-divider">
                <span>{{ t('auth.oauthOrContinue') }}</span>
              </div>

              <EmailOAuthButtons
                :disabled="authActionDisabled"
                :github-enabled="githubOAuthEnabled"
                :google-enabled="googleOAuthEnabled"
                :show-divider="false"
              />

              <LinuxDoOAuthSection
                v-if="linuxdoOAuthEnabled"
                :disabled="authActionDisabled"
                :show-divider="false"
              />
              <DingTalkOAuthSection
                v-if="dingtalkOAuthEnabled"
                :disabled="authActionDisabled"
                :show-divider="false"
              />
              <WechatOAuthSection
                v-if="wechatOAuthEnabled"
                :disabled="authActionDisabled"
                :show-divider="false"
              />
              <OidcOAuthSection
                v-if="oidcOAuthEnabled"
                :disabled="authActionDisabled"
                :provider-name="oidcOAuthProviderName"
                :show-divider="false"
              />
            </div>
          </form>

          <p v-if="!backendModeEnabled" class="md3-register-link">
            {{ t('auth.dontHaveAccount') }}
            <router-link to="/register">
              {{ t('auth.signUp') }}
            </router-link>
          </p>
        </div>
      </section>
    </div>
  </main>

  <!-- 2FA Modal -->
  <TotpLoginModal
    v-if="show2FAModal"
    ref="totpModalRef"
    :temp-token="totpTempToken"
    :user-email-masked="totpUserEmailMasked"
    @verify="handle2FAVerify"
    @cancel="handle2FACancel"
  />
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LinuxDoOAuthSection from '@/components/auth/LinuxDoOAuthSection.vue'
import DingTalkOAuthSection from '@/components/auth/DingTalkOAuthSection.vue'
import OidcOAuthSection from '@/components/auth/OidcOAuthSection.vue'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'
import LoginAgreementPrompt from '@/components/auth/LoginAgreementPrompt.vue'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore, useAppStore } from '@/stores'
import { getPublicSettings, isTotp2FARequired, isWeChatWebOAuthEnabled } from '@/api/auth'
import type { LoginAgreementDocument, TotpLoginResponse } from '@/types'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { clearAllAffiliateReferralCodes } from '@/utils/oauthAffiliate'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const LOGIN_AGREEMENT_STORAGE_KEY = 'sub2api_login_agreement_consent'

// ==================== Router & Stores ====================

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const preferredColorScheme = window.matchMedia('(prefers-color-scheme: dark)')

// ==================== State ====================

const isLoading = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)
const publicSettingsLoaded = ref<boolean>(false)
const isDark = ref<boolean>(document.documentElement.classList.contains('dark'))

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const linuxdoOAuthEnabled = ref<boolean>(false)
const dingtalkOAuthEnabled = ref<boolean>(false)
const wechatOAuthEnabled = ref<boolean>(false)
const backendModeEnabled = ref<boolean>(false)
const oidcOAuthEnabled = ref<boolean>(false)
const oidcOAuthProviderName = ref<string>('OIDC')
const githubOAuthEnabled = ref<boolean>(false)
const googleOAuthEnabled = ref<boolean>(false)
const passwordResetEnabled = ref<boolean>(false)
const loginAgreementEnabled = ref<boolean>(false)
const loginAgreementMode = ref<'modal' | 'checkbox' | string>('modal')
const loginAgreementUpdatedAt = ref<string>('')
const loginAgreementRevision = ref<string>('')
const loginAgreementDocuments = ref<LoginAgreementDocument[]>([])
const agreementAccepted = ref<boolean>(false)
const showAgreementModal = ref<boolean>(false)

// Turnstile
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref<string>('')

// 2FA state
const show2FAModal = ref<boolean>(false)
const totpTempToken = ref<string>('')
const totpUserEmailMasked = ref<string>('')
const totpModalRef = ref<InstanceType<typeof TotpLoginModal> | null>(null)

const formData = reactive({
  email: '',
  password: ''
})

const errors = reactive({
  email: '',
  password: '',
  turnstile: ''
})

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const siteSubtitle = computed(
  () =>
    appStore.cachedPublicSettings?.site_subtitle ||
    t('home.heroSubtitle')
)
const brandInitial = computed(() => siteName.value.trim().charAt(0).toUpperCase() || 'S')
const currentYear = computed(() => new Date().getFullYear())
const contextItems = computed<Array<{ label: string; value: string; icon: 'server' | 'sync' | 'creditCard' }>>(() => [
  {
    label: t('home.features.unifiedGateway'),
    value: t('home.tags.subscriptionToApi'),
    icon: 'server'
  },
  {
    label: t('home.features.multiAccount'),
    value: t('home.tags.stickySession'),
    icon: 'sync'
  },
  {
    label: t('home.features.balanceQuota'),
    value: t('home.tags.realtimeBilling'),
    icon: 'creditCard'
  }
])

const validationToastMessage = computed(
  () => errors.email || errors.password || errors.turnstile || ''
)

const agreementGateActive = computed(
  () => loginAgreementEnabled.value && !agreementAccepted.value
)

const authActionDisabled = computed(
  () => isLoading.value || !publicSettingsLoaded.value || agreementGateActive.value
)

const showOAuthLogin = computed(
  () =>
    !backendModeEnabled.value &&
    (linuxdoOAuthEnabled.value ||
      dingtalkOAuthEnabled.value ||
      wechatOAuthEnabled.value ||
      oidcOAuthEnabled.value ||
      githubOAuthEnabled.value ||
      googleOAuthEnabled.value)
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// ==================== Lifecycle ====================

onMounted(async () => {
  syncThemePreference()
  window.addEventListener('storage', handleThemeStorage)
  preferredColorScheme.addEventListener('change', handleSystemThemeChange)

  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }

  const expiredFlag = sessionStorage.getItem('auth_expired')
  if (expiredFlag) {
    sessionStorage.removeItem('auth_expired')
    const message = t('auth.reloginRequired')
    errorMessage.value = message
    appStore.showWarning(message)
  }

  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled
    dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
    wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
    backendModeEnabled.value = settings.backend_mode_enabled
    oidcOAuthEnabled.value = settings.oidc_oauth_enabled
    oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
    githubOAuthEnabled.value = settings.github_oauth_enabled
    googleOAuthEnabled.value = settings.google_oauth_enabled
    backendModeEnabled.value = settings.backend_mode_enabled
    passwordResetEnabled.value = settings.password_reset_enabled
    applyLoginAgreementSettings(settings)
  } catch (error) {
    console.error('Failed to load public settings:', error)
    loginAgreementEnabled.value = false
    agreementAccepted.value = true
  } finally {
    publicSettingsLoaded.value = true
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('storage', handleThemeStorage)
  preferredColorScheme.removeEventListener('change', handleSystemThemeChange)
})

// ==================== Theme ====================

function resolveThemePreference(): boolean {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark') return true
  if (savedTheme === 'light') return false
  return preferredColorScheme.matches
}

function applyTheme(dark: boolean, persist = true): void {
  isDark.value = dark
  document.documentElement.classList.toggle('dark', dark)

  if (persist) {
    localStorage.setItem('theme', dark ? 'dark' : 'light')
  }
}

function syncThemePreference(): void {
  applyTheme(resolveThemePreference(), false)
}

function toggleTheme(): void {
  applyTheme(!isDark.value)
}

function handleThemeStorage(event: StorageEvent): void {
  if (event.key === 'theme') {
    syncThemePreference()
  }
}

function handleSystemThemeChange(): void {
  if (!localStorage.getItem('theme')) {
    syncThemePreference()
  }
}

// ==================== Login Agreement ====================

function applyLoginAgreementSettings(settings: {
  login_agreement_enabled?: boolean
  login_agreement_mode?: string
  login_agreement_updated_at?: string
  login_agreement_revision?: string
  login_agreement_documents?: LoginAgreementDocument[]
}): void {
  const documents = Array.isArray(settings.login_agreement_documents)
    ? settings.login_agreement_documents.filter((doc) => doc.title?.trim())
    : []
  loginAgreementDocuments.value = documents
  loginAgreementEnabled.value = settings.login_agreement_enabled === true && documents.length > 0
  loginAgreementMode.value = settings.login_agreement_mode === 'checkbox' ? 'checkbox' : 'modal'
  loginAgreementUpdatedAt.value = settings.login_agreement_updated_at || ''
  loginAgreementRevision.value =
    settings.login_agreement_revision ||
    `${loginAgreementUpdatedAt.value}:${documents.map((doc) => `${doc.id}:${doc.title}`).join('|')}`

  agreementAccepted.value = !loginAgreementEnabled.value || hasAcceptedLoginAgreement(loginAgreementRevision.value)
  showAgreementModal.value =
    loginAgreementEnabled.value && !agreementAccepted.value && loginAgreementMode.value !== 'checkbox'
}

function hasAcceptedLoginAgreement(revision: string): boolean {
  if (!revision) {
    return false
  }
  try {
    const raw = localStorage.getItem(LOGIN_AGREEMENT_STORAGE_KEY)
    if (!raw) {
      return false
    }
    const parsed = JSON.parse(raw) as { revision?: string }
    return parsed.revision === revision
  } catch {
    return false
  }
}

function acceptLoginAgreement(): void {
  if (loginAgreementRevision.value) {
    localStorage.setItem(
      LOGIN_AGREEMENT_STORAGE_KEY,
      JSON.stringify({
        revision: loginAgreementRevision.value,
        accepted_at: new Date().toISOString()
      })
    )
  }
  agreementAccepted.value = true
  showAgreementModal.value = false
}

function rejectLoginAgreement(): void {
  localStorage.removeItem(LOGIN_AGREEMENT_STORAGE_KEY)
  agreementAccepted.value = false
  showAgreementModal.value = false
  appStore.showWarning('未同意最新条款前，无法输入账号密码或使用快捷登录。')
}

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string): void {
  turnstileToken.value = token
  errors.turnstile = ''
}

function onTurnstileExpire(): void {
  turnstileToken.value = ''
  errors.turnstile = t('auth.turnstileExpired')
}

function onTurnstileError(): void {
  turnstileToken.value = ''
  errors.turnstile = t('auth.turnstileFailed')
}

// ==================== Validation ====================

function validateForm(): boolean {
  // Reset errors
  errors.email = ''
  errors.password = ''
  errors.turnstile = ''

  let isValid = true

  if (agreementGateActive.value) {
    appStore.showWarning('请先阅读并同意最新条款后再登录。')
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return false
  }

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    errors.email = t('auth.invalidEmail')
    isValid = false
  }

  // Password validation
  if (!formData.password) {
    errors.password = t('auth.passwordRequired')
    isValid = false
  } else if (formData.password.length < 6) {
    errors.password = t('auth.passwordMinLength')
    isValid = false
  }

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

async function handleLogin(): Promise<void> {
  // Clear previous error
  errorMessage.value = ''

  // Validate form
  if (!validateForm()) {
    return
  }

  isLoading.value = true

  try {
    // Call auth store login
    const response = await authStore.login({
      email: formData.email,
      password: formData.password,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined
    })

    // Check if 2FA is required
    if (isTotp2FARequired(response)) {
      const totpResponse = response as TotpLoginResponse
      totpTempToken.value = totpResponse.temp_token || ''
      totpUserEmailMasked.value = totpResponse.user_email_masked || ''
      show2FAModal.value = true
      isLoading.value = false
      return
    }

    // Show success toast
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    // Reset Turnstile on error
    if (turnstileRef.value) {
      turnstileRef.value.reset()
      turnstileToken.value = ''
    }

    errorMessage.value = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.loginFailed'))

    // Also show error toast
    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}

// ==================== 2FA Handlers ====================

async function handle2FAVerify(code: string): Promise<void> {
  if (totpModalRef.value) {
    totpModalRef.value.setVerifying(true)
  }

  try {
    await authStore.login2FA(totpTempToken.value, code)

    // Close modal and show success
    show2FAModal.value = false
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { message?: string } } }
    const message = err.response?.data?.message || err.message || t('profile.totp.loginFailed')

    if (totpModalRef.value) {
      totpModalRef.value.setError(message)
      totpModalRef.value.setVerifying(false)
    }
  }
}

function handle2FACancel(): void {
  show2FAModal.value = false
  totpTempToken.value = ''
  totpUserEmailMasked.value = ''
}
</script>

<style scoped>
.md3-login {
  --md-primary: #006a60;
  --md-on-primary: #ffffff;
  --md-primary-container: #9cf2e4;
  --md-on-primary-container: #00201c;
  --md-secondary-container: #e8def8;
  --md-on-secondary-container: #1d192b;
  --md-surface: #fffbff;
  --md-surface-container-low: #f7f2fa;
  --md-surface-container: #f3edf7;
  --md-surface-container-high: #ece6f0;
  --md-on-surface: #1d1b20;
  --md-on-surface-variant: #49454f;
  --md-outline: #79747e;
  --md-outline-variant: #cac4d0;
  --md-error: #ba1a1a;
  --md-error-container: #ffdad6;
  --md-on-error-container: #410002;
  --md-shadow: 0 1px 2px rgb(0 0 0 / 0.14), 0 1px 3px 1px rgb(0 0 0 / 0.08);
  min-height: 100vh;
  background: var(--md-surface);
  color: var(--md-on-surface);
}

.md3-login-dark {
  --md-primary: #80d5c8;
  --md-on-primary: #003731;
  --md-primary-container: #005048;
  --md-on-primary-container: #9cf2e4;
  --md-secondary-container: #4a4458;
  --md-on-secondary-container: #e8def8;
  --md-surface: #141218;
  --md-surface-container-low: #1d1b20;
  --md-surface-container: #211f26;
  --md-surface-container-high: #2b2930;
  --md-on-surface: #e6e0e9;
  --md-on-surface-variant: #cac4d0;
  --md-outline: #938f99;
  --md-outline-variant: #49454f;
  --md-error: #ffb4ab;
  --md-error-container: #93000a;
  --md-on-error-container: #ffdad6;
  --md-shadow: none;
}

.md3-login-shell {
  display: grid;
  width: min(1180px, calc(100% - 32px));
  min-height: 100vh;
  margin-inline: auto;
  grid-template-columns: minmax(0, 0.95fr) minmax(360px, 440px);
  gap: 48px;
  align-items: center;
  padding: 40px 0;
}

.md3-login-context {
  display: flex;
  min-height: min(720px, calc(100vh - 80px));
  flex-direction: column;
  justify-content: space-between;
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 32px;
}

.md3-brand {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
  color: var(--md-on-surface);
}

.md3-logo {
  display: grid;
  width: 48px;
  height: 48px;
  flex: 0 0 48px;
  place-items: center;
  overflow: hidden;
  border-radius: 8px;
  background: var(--md-primary);
  color: var(--md-on-primary);
  font-size: 1.125rem;
  font-weight: 700;
}

.md3-logo img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.md3-brand-copy {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.md3-brand-copy strong,
.md3-context-item strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-brand-copy strong {
  color: var(--md-on-surface);
  font-size: 1rem;
  font-weight: 700;
}

.md3-brand-copy span,
.md3-copyright,
.md3-context-item small {
  color: var(--md-on-surface-variant);
}

.md3-brand-copy span {
  overflow: hidden;
  max-width: 34rem;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.875rem;
}

.md3-context-copy {
  max-width: 560px;
}

.md3-chip {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container);
  padding: 0 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  font-weight: 600;
}

.md3-context-copy h1 {
  margin-top: 24px;
  color: var(--md-on-surface);
  font-size: clamp(2.5rem, 8vw, 5.75rem);
  font-weight: 700;
  line-height: 0.96;
}

.md3-context-copy p {
  margin-top: 18px;
  max-width: 500px;
  color: var(--md-on-surface-variant);
  font-size: 1rem;
  line-height: 1.8;
}

.md3-context-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.md3-context-item {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface);
  padding: 14px;
}

.md3-context-item > span {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 8px;
  background: var(--md-primary-container);
  color: var(--md-on-primary-container);
}

.md3-context-item div {
  min-width: 0;
}

.md3-context-item strong {
  display: block;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 700;
}

.md3-context-item small {
  display: block;
  margin-top: 2px;
  font-size: 0.75rem;
}

.md3-copyright {
  font-size: 0.75rem;
}

.md3-login-panel {
  min-width: 0;
}

.md3-panel-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 16px;
}

.md3-icon-button {
  display: inline-grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: 20px;
  color: var(--md-on-surface-variant);
  transition: background-color 160ms ease, color 160ms ease;
}

.md3-icon-button:hover {
  background: color-mix(in srgb, var(--md-on-surface) 8%, transparent);
  color: var(--md-on-surface);
}

.md3-locale :deep(button) {
  height: 40px;
  border-radius: 20px;
  color: var(--md-on-surface-variant);
}

.md3-locale :deep(button:hover) {
  background: color-mix(in srgb, var(--md-on-surface) 8%, transparent);
}

.md3-auth-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  box-shadow: var(--md-shadow);
  padding: 32px;
}

.md3-card-heading span {
  color: var(--md-primary);
  font-size: 0.8125rem;
  font-weight: 700;
}

.md3-card-heading h2 {
  margin-top: 6px;
  color: var(--md-on-surface);
  font-size: 2rem;
  font-weight: 700;
  line-height: 1.15;
}

.md3-card-heading p {
  margin-top: 8px;
  color: var(--md-on-surface-variant);
  font-size: 0.9375rem;
  line-height: 1.6;
}

.md3-form {
  display: grid;
  gap: 18px;
  margin-top: 28px;
}

.md3-field {
  display: grid;
  gap: 8px;
}

.md3-field label,
.md3-label-row label {
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 600;
}

.md3-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.md3-text-link,
.md3-register-link a {
  color: var(--md-primary);
  font-size: 0.875rem;
  font-weight: 700;
  text-decoration: none;
}

.md3-text-link:hover,
.md3-register-link a:hover {
  text-decoration: underline;
  text-underline-offset: 4px;
}

.md3-input-shell {
  display: flex;
  min-height: 56px;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--md-outline);
  border-radius: 8px;
  background: var(--md-surface);
  padding: 0 14px;
  color: var(--md-on-surface-variant);
  transition: border-color 160ms ease, box-shadow 160ms ease, background-color 160ms ease;
}

.md3-input-shell:focus-within {
  border-color: var(--md-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--md-primary) 22%, transparent);
}

.md3-field-error .md3-input-shell {
  border-color: var(--md-error);
}

.md3-input-icon {
  flex: 0 0 auto;
}

.md3-input-shell input {
  min-width: 0;
  flex: 1;
  border: 0;
  background: transparent;
  color: var(--md-on-surface);
  font-size: 0.9375rem;
  outline: none;
}

.md3-input-shell input::placeholder {
  color: color-mix(in srgb, var(--md-on-surface-variant) 72%, transparent);
}

.md3-input-shell input:disabled {
  cursor: not-allowed;
}

.md3-password-button {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 18px;
  color: var(--md-on-surface-variant);
  transition: background-color 160ms ease, color 160ms ease;
}

.md3-password-button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--md-on-surface) 8%, transparent);
  color: var(--md-on-surface);
}

.md3-password-button:disabled,
.md3-primary-button:disabled {
  cursor: not-allowed;
  opacity: 0.58;
}

.md3-error-message {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  border-radius: 8px;
  background: var(--md-error-container);
  padding: 12px;
  color: var(--md-on-error-container);
  font-size: 0.875rem;
  line-height: 1.5;
}

.md3-turnstile {
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface);
  padding: 10px;
}

.md3-primary-button {
  display: inline-flex;
  min-height: 48px;
  width: 100%;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border-radius: 24px;
  background: var(--md-primary);
  color: var(--md-on-primary);
  font-size: 0.9375rem;
  font-weight: 700;
  box-shadow: var(--md-shadow);
  transition: background-color 160ms ease, box-shadow 160ms ease, transform 160ms ease;
}

.md3-primary-button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--md-primary) 92%, var(--md-on-primary));
  box-shadow: 0 2px 6px rgb(0 0 0 / 0.16);
}

.md3-primary-button:active:not(:disabled) {
  transform: translateY(1px);
}

.md3-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: md3-login-spin 700ms linear infinite;
}

.md3-agreement:empty {
  display: none;
}

.md3-oauth-stack {
  display: grid;
  gap: 12px;
}

.md3-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.8125rem;
  font-weight: 600;
}

.md3-divider::before,
.md3-divider::after {
  height: 1px;
  flex: 1;
  background: var(--md-outline-variant);
  content: '';
}

.md3-oauth-stack :deep(.btn.btn-secondary) {
  min-height: 48px;
  width: 100%;
  border: 1px solid var(--md-outline-variant);
  border-radius: 24px;
  background: var(--md-secondary-container);
  color: var(--md-on-secondary-container);
  box-shadow: none;
  font-size: 0.875rem;
  font-weight: 700;
  transition: background-color 160ms ease, border-color 160ms ease;
}

.md3-oauth-stack :deep(.btn.btn-secondary:hover:not(:disabled)) {
  border-color: var(--md-outline);
  background: color-mix(in srgb, var(--md-secondary-container) 88%, var(--md-on-secondary-container));
}

.md3-oauth-stack :deep(.btn.btn-secondary:disabled) {
  cursor: not-allowed;
  opacity: 0.58;
}

.md3-oauth-stack :deep(.space-y-4),
.md3-oauth-stack :deep(.space-y-3) {
  display: grid;
  gap: 12px;
}

.md3-oauth-stack :deep(.space-y-4 > :not([hidden]) ~ :not([hidden])),
.md3-oauth-stack :deep(.space-y-3 > :not([hidden]) ~ :not([hidden])) {
  margin-top: 0;
}

.md3-oauth-stack :deep(.grid) {
  gap: 12px;
}

.md3-oauth-stack :deep([data-testid='wechat-oauth-hint']) {
  color: var(--md-on-surface-variant);
  font-size: 0.8125rem;
}

.md3-register-link {
  margin-top: 22px;
  color: var(--md-on-surface-variant);
  text-align: center;
  font-size: 0.875rem;
}

@keyframes md3-login-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 980px) {
  .md3-login-shell {
    grid-template-columns: minmax(0, 1fr);
    gap: 24px;
    align-items: start;
    padding: 24px 0;
  }

  .md3-login-context {
    min-height: auto;
    gap: 28px;
  }

  .md3-context-copy h1 {
    font-size: clamp(2.25rem, 12vw, 4.5rem);
  }

  .md3-context-list {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .md3-login-shell {
    width: min(100% - 24px, 520px);
    padding: 12px 0 24px;
  }

  .md3-login-context {
    padding: 20px;
  }

  .md3-brand-copy span,
  .md3-copyright,
  .md3-context-list {
    display: none;
  }

  .md3-context-copy {
    margin-top: 4px;
  }

  .md3-context-copy h1 {
    margin-top: 18px;
  }

  .md3-context-copy p {
    font-size: 0.9375rem;
  }

  .md3-auth-card {
    padding: 24px 18px;
  }

  .md3-card-heading h2 {
    font-size: 1.75rem;
  }

  .md3-label-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }
}
</style>
