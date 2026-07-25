<template>
  <AuthPortalShell :site-name="siteName" wide>
    <template #default="{ isDark }">
      <div class="auth-portal-heading">
        <h1 id="register-title">{{ t('auth.createAccount') }}</h1>
        <p>{{ t('auth.signUpToStart', { siteName }) }}</p>
      </div>

      <div
        v-if="!registrationEnabled && settingsLoaded"
        class="auth-portal-alert auth-portal-alert--warning auth-portal-register-notice"
        role="status"
      >
        <Icon name="exclamationCircle" size="sm" />
        <span>{{ t('auth.registrationDisabled') }}</span>
      </div>

      <form v-else class="auth-portal-form" @submit.prevent="handleRegister">
        <div class="auth-portal-field" :class="{ 'auth-portal-field--error': errors.email }">
          <label for="email">{{ t('auth.emailLabel') }}</label>
          <div class="auth-portal-input">
            <Icon name="mail" size="sm" />
            <input
              id="email"
              v-model="formData.email"
              type="email"
              required
              autofocus
              autocomplete="email"
              :disabled="registrationActionDisabled"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </div>
          <p v-if="errors.email" class="auth-portal-field-message auth-portal-field-message--error">
            {{ errors.email }}
          </p>
        </div>

        <div class="auth-portal-field" :class="{ 'auth-portal-field--error': errors.password }">
          <label for="password">{{ t('auth.passwordLabel') }}</label>
          <div class="auth-portal-input">
            <Icon name="lock" size="sm" />
            <input
              id="password"
              v-model="formData.password"
              :type="showPassword ? 'text' : 'password'"
              required
              autocomplete="new-password"
              :disabled="registrationActionDisabled"
              :placeholder="t('auth.createPasswordPlaceholder')"
            />
            <button
              type="button"
              class="auth-portal-input-action"
              :disabled="registrationActionDisabled"
              :aria-label="showPassword ? 'Hide password' : 'Show password'"
              @click="showPassword = !showPassword"
            >
              <Icon :name="showPassword ? 'eyeOff' : 'eye'" size="sm" />
            </button>
          </div>
          <p
            v-if="errors.password"
            class="auth-portal-field-message auth-portal-field-message--error"
          >
            {{ errors.password }}
          </p>
          <p v-else class="auth-portal-hint">{{ t('auth.passwordHint') }}</p>
        </div>

        <div
          v-if="invitationCodeEnabled"
          class="auth-portal-field"
          :class="{ 'auth-portal-field--error': invitationValidation.invalid || errors.invitation_code }"
        >
          <label for="invitation_code">{{ t('auth.invitationCodeLabel') }}</label>
          <div class="auth-portal-input">
            <Icon name="key" size="sm" />
            <input
              id="invitation_code"
              v-model="formData.invitation_code"
              type="text"
              :disabled="registrationActionDisabled"
              :placeholder="t('auth.invitationCodePlaceholder')"
              @input="handleInvitationCodeInput"
            />
            <span v-if="invitationValidating" class="auth-portal-spinner auth-portal-validation-icon"></span>
            <Icon
              v-else-if="invitationValidation.valid"
              name="checkCircle"
              size="sm"
              class="auth-portal-validation-icon auth-portal-field-message--success"
            />
            <Icon
              v-else-if="invitationValidation.invalid || errors.invitation_code"
              name="exclamationCircle"
              size="sm"
              class="auth-portal-validation-icon auth-portal-field-message--error"
            />
          </div>
          <p
            v-if="invitationValidation.valid"
            class="auth-portal-field-message auth-portal-field-message--success"
          >
            {{ t('auth.invitationCodeValid') }}
          </p>
          <p
            v-else-if="errors.invitation_code || invitationValidation.message"
            class="auth-portal-field-message auth-portal-field-message--error"
          >
            {{ errors.invitation_code || invitationValidation.message }}
          </p>
        </div>

        <div
          v-if="promoCodeEnabled"
          class="auth-portal-field"
          :class="{ 'auth-portal-field--error': promoValidation.invalid }"
        >
          <label for="promo_code">
            {{ t('auth.promoCodeLabel') }}
            <span class="auth-portal-optional">{{ t('common.optional') }}</span>
          </label>
          <div class="auth-portal-input">
            <Icon name="gift" size="sm" />
            <input
              id="promo_code"
              v-model="formData.promo_code"
              type="text"
              :disabled="registrationActionDisabled"
              :placeholder="t('auth.promoCodePlaceholder')"
              @input="handlePromoCodeInput"
            />
            <span v-if="promoValidating" class="auth-portal-spinner auth-portal-validation-icon"></span>
            <Icon
              v-else-if="promoValidation.valid"
              name="checkCircle"
              size="sm"
              class="auth-portal-validation-icon auth-portal-field-message--success"
            />
            <Icon
              v-else-if="promoValidation.invalid"
              name="exclamationCircle"
              size="sm"
              class="auth-portal-validation-icon auth-portal-field-message--error"
            />
          </div>
          <p
            v-if="promoValidation.valid"
            class="auth-portal-field-message auth-portal-field-message--success"
          >
            {{ promoCodeSuccessMessage }}
          </p>
          <p
            v-else-if="promoValidation.message"
            class="auth-portal-field-message auth-portal-field-message--error"
          >
            {{ promoValidation.message }}
          </p>
        </div>

        <div
          v-if="affiliateEnabled"
          class="auth-portal-field"
          :class="{ 'auth-portal-field--error': affValidation.invalid }"
        >
          <label for="aff_code">
            {{ t('auth.affCodeLabel') }}
            <span class="auth-portal-optional">{{ t('common.optional') }}</span>
          </label>
          <div class="auth-portal-input">
            <Icon name="users" size="sm" />
            <input
              id="aff_code"
              v-model="formData.aff_code"
              type="text"
              :disabled="registrationActionDisabled"
              :placeholder="t('auth.affCodePlaceholder')"
              @input="handleAffCodeInput"
            />
            <span v-if="affValidating" class="auth-portal-spinner auth-portal-validation-icon"></span>
            <Icon
              v-else-if="affValidation.valid"
              name="checkCircle"
              size="sm"
              class="auth-portal-validation-icon auth-portal-field-message--success"
            />
            <Icon
              v-else-if="affValidation.invalid"
              name="exclamationCircle"
              size="sm"
              class="auth-portal-validation-icon auth-portal-field-message--error"
            />
          </div>
          <p
            v-if="affValidation.valid"
            class="auth-portal-field-message auth-portal-field-message--success"
          >
            {{ t('auth.affCodeValid') }}
          </p>
          <p
            v-else-if="affValidation.message"
            class="auth-portal-field-message auth-portal-field-message--error"
          >
            {{ affValidation.message }}
          </p>
        </div>

        <div v-if="errorMessage" class="auth-portal-alert" role="alert">
          <Icon name="exclamationCircle" size="sm" />
          <span>{{ errorMessage }}</span>
        </div>

        <div v-if="turnstileEnabled && turnstileSiteKey" class="auth-portal-turnstile">
          <TurnstileWidget
            ref="turnstileRef"
            :site-key="turnstileSiteKey"
            :theme="isDark ? 'dark' : 'light'"
            @verify="onTurnstileVerify"
            @expire="onTurnstileExpire"
            @error="onTurnstileError"
          />
        </div>

        <div class="auth-portal-agreement">
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

        <button
          type="submit"
          class="auth-portal-primary"
          :disabled="registrationActionDisabled || (turnstileEnabled && !turnstileToken)"
        >
          <span v-if="isLoading" class="auth-portal-spinner" aria-hidden="true"></span>
          <span>
            {{
              isLoading
                ? t('auth.processing')
                : emailVerifyEnabled
                  ? t('auth.continue')
                  : t('auth.createAccount')
            }}
          </span>
          <Icon v-if="!isLoading" name="arrowRight" size="sm" />
        </button>

        <div v-if="showOAuthLogin" class="auth-portal-oauth">
          <div class="auth-portal-divider">
            <span>{{ t('auth.oauthOrContinue') }}</span>
          </div>
          <EmailOAuthButtons
            :disabled="registrationActionDisabled"
            :aff-code="formData.aff_code"
            :github-enabled="githubOAuthEnabled"
            :google-enabled="googleOAuthEnabled"
            :show-divider="false"
          />
          <LinuxDoOAuthSection
            v-if="linuxdoOAuthEnabled"
            :disabled="registrationActionDisabled"
            :aff-code="formData.aff_code"
            :show-divider="false"
          />
          <WechatOAuthSection
            v-if="wechatOAuthEnabled"
            :disabled="registrationActionDisabled"
            :aff-code="formData.aff_code"
            :show-divider="false"
          />
          <OidcOAuthSection
            v-if="oidcOAuthEnabled"
            :disabled="registrationActionDisabled"
            :provider-name="oidcOAuthProviderName"
            :aff-code="formData.aff_code"
            :show-divider="false"
          />
        </div>
      </form>

      <p class="auth-portal-switch">
        {{ t('auth.alreadyHaveAccount') }}
        <router-link to="/login">{{ t('auth.signIn') }}</router-link>
      </p>
    </template>
  </AuthPortalShell>
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, onUnmounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AuthPortalShell from '@/components/auth/AuthPortalShell.vue'
import LinuxDoOAuthSection from '@/components/auth/LinuxDoOAuthSection.vue'
import OidcOAuthSection from '@/components/auth/OidcOAuthSection.vue'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'
import LoginAgreementPrompt from '@/components/auth/LoginAgreementPrompt.vue'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { useAuthStore, useAppStore } from '@/stores'
import {
  getPublicSettings,
  isWeChatWebOAuthEnabled,
  validateAffCode,
  validatePromoCode,
  validateInvitationCode
} from '@/api/auth'
import { buildAuthErrorMessage } from '@/utils/authError'
import {
  formatRegistrationEmailSuffixWhitelistForMessage,
  isRegistrationEmailSuffixAllowed,
  normalizeRegistrationEmailSuffixWhitelist
} from '@/utils/registrationEmailPolicy'
import {
  clearAffiliateReferralCode,
  loadAffiliateReferralCode,
  resolveAffiliateReferralCode
} from '@/utils/oauthAffiliate'
import type { LoginAgreementDocument } from '@/types'

const { t, locale } = useI18n()
const LOGIN_AGREEMENT_STORAGE_KEY = 'sub2api_login_agreement_consent'

// ==================== Router & Stores ====================

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const settingsLoaded = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)

// Public settings
const registrationEnabled = ref<boolean>(true)
const emailVerifyEnabled = ref<boolean>(false)
const promoCodeEnabled = ref<boolean>(true)
const affiliateEnabled = ref<boolean>(false)
const invitationCodeEnabled = ref<boolean>(false)
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const siteName = ref<string>('Sub2API')
const linuxdoOAuthEnabled = ref<boolean>(false)
const wechatOAuthEnabled = ref<boolean>(false)
const oidcOAuthEnabled = ref<boolean>(false)
const oidcOAuthProviderName = ref<string>('OIDC')
const githubOAuthEnabled = ref<boolean>(false)
const googleOAuthEnabled = ref<boolean>(false)
const registrationEmailSuffixWhitelist = ref<string[]>([])
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

// Promo code validation
const promoValidating = ref<boolean>(false)
const promoValidation = reactive({
  valid: false,
  invalid: false,
  bonusAmount: null as number | null,
  message: ''
})
let promoValidateTimeout: ReturnType<typeof setTimeout> | null = null

// Affiliate code validation
const affValidating = ref<boolean>(false)
const affValidation = reactive({
  valid: false,
  invalid: false,
  message: ''
})
let affValidateTimeout: ReturnType<typeof setTimeout> | null = null

// Invitation code validation
const invitationValidating = ref<boolean>(false)
const invitationValidation = reactive({
  valid: false,
  invalid: false,
  message: ''
})
let invitationValidateTimeout: ReturnType<typeof setTimeout> | null = null

const formData = reactive({
  email: '',
  password: '',
  promo_code: '',
  invitation_code: '',
  aff_code: ''
})

const errors = reactive({
  email: '',
  password: '',
  turnstile: '',
  invitation_code: ''
})

const validationToastMessage = computed(() =>
  errors.email ||
  errors.password ||
  (invitationValidation.invalid ? invitationValidation.message : '') ||
  errors.invitation_code ||
  (promoValidation.invalid ? promoValidation.message : '') ||
  (affValidation.invalid ? affValidation.message : '') ||
  errors.turnstile ||
  ''
)

const promoCodeSuccessMessage = computed(() => {
  if (!promoValidation.valid) {
    return ''
  }
  const bonusAmount = promoValidation.bonusAmount || 0
  if (bonusAmount <= 0) {
    return t('auth.promoCodeValidNoBonus')
  }
  return t('auth.promoCodeValid', { amount: bonusAmount.toFixed(2) })
})

const showOAuthLogin = computed(
  () =>
    linuxdoOAuthEnabled.value ||
    wechatOAuthEnabled.value ||
    oidcOAuthEnabled.value ||
    githubOAuthEnabled.value ||
    googleOAuthEnabled.value
)

const agreementGateActive = computed(
  () => loginAgreementEnabled.value && !agreementAccepted.value
)

const registrationActionDisabled = computed(
  () => isLoading.value || !settingsLoaded.value || agreementGateActive.value
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

function syncAffiliateReferralCode(): string {
  const code = resolveAffiliateReferralCode(route.query.aff, route.query.aff_code)
  if (code) {
    formData.aff_code = code
  }
  return code
}

function routeStringParam(value: unknown): string {
  const raw = Array.isArray(value) ? value[0] : value
  return typeof raw === 'string' ? raw.trim() : ''
}

function markAffCodeValid(code: string): void {
  formData.aff_code = code
  affValidation.valid = true
  affValidation.invalid = false
  affValidation.message = ''
}

function markPromoCodeValid(code: string, bonusAmount: number | null = null): void {
  formData.promo_code = code
  promoValidation.valid = true
  promoValidation.invalid = false
  promoValidation.bonusAmount = bonusAmount
  promoValidation.message = ''
}

// ==================== Lifecycle ====================

onMounted(async () => {
  syncAffiliateReferralCode()

  try {
    const settings = await getPublicSettings()
    registrationEnabled.value = settings.registration_enabled
    emailVerifyEnabled.value = settings.email_verify_enabled
    promoCodeEnabled.value = settings.promo_code_enabled
    affiliateEnabled.value = settings.affiliate_enabled
    invitationCodeEnabled.value = settings.invitation_code_enabled
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    siteName.value = settings.site_name || 'Sub2API'
    linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled
    wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
    oidcOAuthEnabled.value = settings.oidc_oauth_enabled
    oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
    githubOAuthEnabled.value = settings.github_oauth_enabled
    googleOAuthEnabled.value = settings.google_oauth_enabled
    registrationEmailSuffixWhitelist.value = normalizeRegistrationEmailSuffixWhitelist(
      settings.registration_email_suffix_whitelist || []
    )
    applyLoginAgreementSettings(settings)

    const affCode = syncAffiliateReferralCode()
    const promoParam = routeStringParam(route.query.promo)
    if (affCode && affiliateEnabled.value) {
      markAffCodeValid(affCode)
    }
    if (promoCodeEnabled.value && promoParam) {
      formData.promo_code = promoParam
      await validatePromoCodeDebounced(promoParam)
    }
  } catch (error) {
    console.error('Failed to load public settings:', error)
    loginAgreementEnabled.value = false
    agreementAccepted.value = true
  } finally {
    settingsLoaded.value = true
  }
})

watch(
  () => [route.query.aff, route.query.aff_code],
  () => {
    const code = syncAffiliateReferralCode()
    if (code && affiliateEnabled.value) {
      markAffCodeValid(code)
    }
  }
)

onUnmounted(() => {
  if (promoValidateTimeout) {
    clearTimeout(promoValidateTimeout)
  }
  if (affValidateTimeout) {
    clearTimeout(affValidateTimeout)
  }
  if (invitationValidateTimeout) {
    clearTimeout(invitationValidateTimeout)
  }
})

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
  appStore.showWarning(t('legal.loginAgreementPrompt.registerRejectedWarning'))
}

// ==================== Promo Code Validation ====================

function handlePromoCodeInput(): void {
  const code = formData.promo_code.trim()

  promoValidation.valid = false
  promoValidation.invalid = false
  promoValidation.bonusAmount = null
  promoValidation.message = ''

  if (promoValidateTimeout) {
    clearTimeout(promoValidateTimeout)
    promoValidateTimeout = null
  }

  if (!code) {
    promoValidating.value = false
    return
  }

  promoValidateTimeout = setTimeout(() => {
    validatePromoCodeDebounced(code)
  }, 500)
}

void handlePromoCodeInput

async function validatePromoCodeDebounced(code: string): Promise<void> {
  const trimmedCode = code.trim()
  if (!trimmedCode) return

  promoValidating.value = true

  try {
    const result = await validatePromoCode(trimmedCode)

    if (formData.promo_code.trim() !== trimmedCode) {
      return
    }

    if (result.valid) {
      markPromoCodeValid(trimmedCode, result.bonus_amount || 0)
    } else {
      promoValidation.valid = false
      promoValidation.invalid = true
      promoValidation.bonusAmount = null
      // 根据错误码显示对应的翻译
      promoValidation.message = getPromoErrorMessage(result.error_code)
    }
  } catch (error) {
    console.error('Failed to validate promo code:', error)
    if (formData.promo_code.trim() !== trimmedCode) {
      return
    }
    promoValidation.valid = false
    promoValidation.invalid = true
    promoValidation.message = t('auth.promoCodeInvalid')
  } finally {
    if (formData.promo_code.trim() === trimmedCode) {
      promoValidating.value = false
    }
  }
}

function getPromoErrorMessage(errorCode?: string): string {
  switch (errorCode) {
    case 'PROMO_CODE_NOT_FOUND':
      return t('auth.promoCodeNotFound')
    case 'PROMO_CODE_EXPIRED':
      return t('auth.promoCodeExpired')
    case 'PROMO_CODE_DISABLED':
      return t('auth.promoCodeDisabled')
    case 'PROMO_CODE_MAX_USED':
      return t('auth.promoCodeMaxUsed')
    case 'PROMO_CODE_ALREADY_USED':
      return t('auth.promoCodeAlreadyUsed')
    default:
      return t('auth.promoCodeInvalid')
  }
}

// ==================== Affiliate Code Validation ====================

function handleAffCodeInput(): void {
  const code = formData.aff_code.trim()

  affValidation.valid = false
  affValidation.invalid = false
  affValidation.message = ''

  if (affValidateTimeout) {
    clearTimeout(affValidateTimeout)
    affValidateTimeout = null
  }

  if (!code) {
    affValidating.value = false
    return
  }

  affValidateTimeout = setTimeout(() => {
    validateAffCodeDebounced(code)
  }, 500)
}

async function validateAffCodeDebounced(code: string): Promise<void> {
  const trimmedCode = code.trim()
  if (!trimmedCode) return

  affValidating.value = true

  try {
    const result = await validateAffCode(trimmedCode)

    if (formData.aff_code.trim() !== trimmedCode) {
      return
    }

    if (result.valid) {
      markAffCodeValid(trimmedCode)
    } else {
      affValidation.valid = false
      affValidation.invalid = true
      affValidation.message = t('auth.affCodeInvalid')
    }
  } catch (error) {
    console.error('Failed to validate affiliate code:', error)
    if (formData.aff_code.trim() !== trimmedCode) {
      return
    }
    affValidation.valid = false
    affValidation.invalid = true
    affValidation.message = t('auth.affCodeInvalid')
  } finally {
    if (formData.aff_code.trim() === trimmedCode) {
      affValidating.value = false
    }
  }
}

// ==================== Invitation Code Validation ====================

function handleInvitationCodeInput(): void {
  const code = formData.invitation_code.trim()

  // Clear previous validation
  invitationValidation.valid = false
  invitationValidation.invalid = false
  invitationValidation.message = ''
  errors.invitation_code = ''

  if (!code) {
    return
  }

  // Debounce validation
  if (invitationValidateTimeout) {
    clearTimeout(invitationValidateTimeout)
  }

  invitationValidateTimeout = setTimeout(() => {
    validateInvitationCodeDebounced(code)
  }, 500)
}

async function validateInvitationCodeDebounced(code: string): Promise<void> {
  invitationValidating.value = true

  try {
    const result = await validateInvitationCode(code)

    if (result.valid) {
      invitationValidation.valid = true
      invitationValidation.invalid = false
      invitationValidation.message = ''
    } else {
      invitationValidation.valid = false
      invitationValidation.invalid = true
      invitationValidation.message = getInvitationErrorMessage(result.error_code)
    }
  } catch {
    invitationValidation.valid = false
    invitationValidation.invalid = true
    invitationValidation.message = t('auth.invitationCodeInvalid')
  } finally {
    invitationValidating.value = false
  }
}

function getInvitationErrorMessage(errorCode?: string): string {
  switch (errorCode) {
    case 'INVITATION_CODE_NOT_FOUND':
      return t('auth.invitationCodeInvalid')
    case 'INVITATION_CODE_INVALID':
      return t('auth.invitationCodeInvalid')
    case 'INVITATION_CODE_USED':
      return t('auth.invitationCodeInvalid')
    case 'INVITATION_CODE_DISABLED':
      return t('auth.invitationCodeInvalid')
    default:
      return t('auth.invitationCodeInvalid')
  }
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

function validateEmail(email: string): boolean {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
  return emailRegex.test(email)
}

function buildEmailSuffixNotAllowedMessage(): string {
  const normalizedWhitelist = normalizeRegistrationEmailSuffixWhitelist(
    registrationEmailSuffixWhitelist.value
  )
  if (normalizedWhitelist.length === 0) {
    return t('auth.emailSuffixNotAllowed')
  }
  const separator = String(locale.value || '').toLowerCase().startsWith('zh') ? '、' : ', '
  return t('auth.emailSuffixNotAllowedWithAllowed', {
    suffixes: formatRegistrationEmailSuffixWhitelistForMessage(normalizedWhitelist, {
      separator,
      more: (count) => t('auth.emailSuffixAllowedMore', { count })
    })
  })
}

function validateForm(): boolean {
  // Reset errors
  errors.email = ''
  errors.password = ''
  errors.turnstile = ''
  errors.invitation_code = ''

  let isValid = true

  if (agreementGateActive.value) {
    appStore.showWarning(t('legal.loginAgreementPrompt.registerRequiredWarning'))
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return false
  }

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  } else if (!validateEmail(formData.email)) {
    errors.email = t('auth.invalidEmail')
    isValid = false
  } else if (
    !isRegistrationEmailSuffixAllowed(formData.email, registrationEmailSuffixWhitelist.value)
  ) {
    errors.email = buildEmailSuffixNotAllowedMessage()
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

  // Invitation code validation (required when enabled)
  if (invitationCodeEnabled.value) {
    if (!formData.invitation_code.trim()) {
      errors.invitation_code = t('auth.invitationCodeRequired')
      isValid = false
    }
  }

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

async function handleRegister(): Promise<void> {
  // Clear previous error
  errorMessage.value = ''

  // Validate form
  if (!validateForm()) {
    return
  }

  const currentPromoCode = formData.promo_code.trim()
  const currentAffCode = formData.aff_code.trim()

  // Check promo code validation status
  if (currentPromoCode) {
    if (promoValidating.value) {
      errorMessage.value = t('auth.promoCodeValidating')
      return
    }
    if (!promoValidation.valid) {
      errorMessage.value = t('auth.promoCodeValidating')
      await validatePromoCodeDebounced(currentPromoCode)
      if (!promoValidation.valid) {
        errorMessage.value = t('auth.promoCodeInvalidCannotRegister')
        return
      }
    }
  }

  // Check affiliate code validation status
  if (currentAffCode && affiliateEnabled.value) {
    if (affValidating.value) {
      errorMessage.value = t('auth.affCodeValidating')
      return
    }
    if (!affValidation.valid) {
      errorMessage.value = t('auth.affCodeValidating')
      await validateAffCodeDebounced(currentAffCode)
      if (!affValidation.valid) {
        errorMessage.value = t('auth.affCodeInvalidCannotRegister')
        return
      }
    }
  }

  // Check invitation code validation status (if enabled and code provided)
  if (invitationCodeEnabled.value) {
    // If still validating, wait
    if (invitationValidating.value) {
      errorMessage.value = t('auth.invitationCodeValidating')
      return
    }
    // If invitation code is invalid, block submission
    if (invitationValidation.invalid) {
      errorMessage.value = t('auth.invitationCodeInvalidCannotRegister')
      return
    }
    // If invitation code is required but not validated yet
    if (formData.invitation_code.trim() && !invitationValidation.valid) {
      errorMessage.value = t('auth.invitationCodeValidating')
      // Trigger validation
      await validateInvitationCodeDebounced(formData.invitation_code.trim())
      if (!invitationValidation.valid) {
        errorMessage.value = t('auth.invitationCodeInvalidCannotRegister')
        return
      }
    }
  }

  isLoading.value = true

  try {
    const affCode = currentAffCode || loadAffiliateReferralCode()
    const promoCode = currentPromoCode
    if (affCode) {
      formData.aff_code = affCode
    }
    if (promoCode) {
      formData.promo_code = promoCode
    }

    // If email verification is enabled, redirect to verification page
    if (emailVerifyEnabled.value) {
      // Store registration data in sessionStorage
      sessionStorage.setItem(
        'register_data',
        JSON.stringify({
          email: formData.email,
          password: formData.password,
          turnstile_token: turnstileToken.value,
          promo_code: promoCode || undefined,
          invitation_code: formData.invitation_code || undefined,
          ...(affCode ? { aff_code: affCode } : {})
        })
      )

      // Navigate to email verification page
      await router.push('/email-verify')
      return
    }

    // Otherwise, directly register
    await authStore.register({
      email: formData.email,
      password: formData.password,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined,
      promo_code: promoCode || undefined,
      invitation_code: formData.invitation_code || undefined,
      ...(affCode ? { aff_code: affCode } : {})
    })
    clearAffiliateReferralCode()

    // Show success toast
    appStore.showSuccess(t('auth.accountCreatedSuccess', { siteName: siteName.value }))

    // Redirect to dashboard
    await router.push('/dashboard')
  } catch (error: unknown) {
    // Reset Turnstile on error
    if (turnstileRef.value) {
      turnstileRef.value.reset()
      turnstileToken.value = ''
    }

    // Handle registration error
    errorMessage.value = buildAuthErrorMessage(error, {
      fallback: t('auth.registrationFailed')
    })

    // Also show error toast
    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}
</script>

<style scoped>
.auth-portal-register-notice {
  margin-top: 32px;
}

.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
