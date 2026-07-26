<template>
  <AppLayout>
    <div
      data-testid="profile-shell"
      class="console-page profile-settings-page"
    >
      <header class="profile-settings-header">
        <p class="profile-settings-kicker">ACCOUNT / SETTINGS</p>
        <h1 class="profile-settings-title">{{ t('profile.title') }}</h1>
        <p class="profile-settings-description">{{ t('profile.description') }}</p>
      </header>

      <div class="profile-settings-grid">
        <nav class="profile-settings-nav" aria-label="Settings sections">
          <button
            type="button"
            class="profile-settings-nav-link"
            :class="{ 'profile-settings-nav-link-active': activeTab === 'profile' }"
            :aria-current="activeTab === 'profile' ? 'page' : undefined"
            @click="selectTab('profile')"
          >{{ t('profile.navProfile') }}</button>
          <button
            type="button"
            class="profile-settings-nav-link"
            :class="{ 'profile-settings-nav-link-active': activeTab === 'referrals' }"
            :aria-current="activeTab === 'referrals' ? 'page' : undefined"
            @click="selectTab('referrals')"
          >{{ t('profile.referralCodesTitle') }}</button>
          <button
            type="button"
            class="profile-settings-nav-link"
            :class="{ 'profile-settings-nav-link-active': activeTab === 'security' }"
            :aria-current="activeTab === 'security' ? 'page' : undefined"
            @click="selectTab('security')"
          >{{ t('profile.navSecurity') }}</button>
          <button
            v-if="user && balanceLowNotifyEnabled"
            type="button"
            class="profile-settings-nav-link"
            :class="{ 'profile-settings-nav-link-active': activeTab === 'notifications' }"
            :aria-current="activeTab === 'notifications' ? 'page' : undefined"
            @click="selectTab('notifications')"
          >{{ t('profile.navNotifications') }}</button>
          <button
            type="button"
            class="profile-settings-nav-link profile-settings-nav-link-danger"
            :class="{ 'profile-settings-nav-link-active': activeTab === 'account' }"
            :aria-current="activeTab === 'account' ? 'page' : undefined"
            @click="selectTab('account')"
          >{{ t('profile.navAccount') }}</button>
        </nav>

        <div class="profile-settings-content">
          <section v-if="activeTab === 'profile'" class="profile-settings-section">
            <h2 class="profile-settings-section-title">{{ t('profile.navProfile') }}</h2>
            <ProfileInfoCard
              :user="user"
              :linuxdo-enabled="linuxdoOAuthEnabled"
              :dingtalk-enabled="dingtalkOAuthEnabled"
              :oidc-enabled="oidcOAuthEnabled"
              :oidc-provider-name="oidcOAuthProviderName"
              :wechat-enabled="wechatOAuthEnabled"
              :wechat-open-enabled="wechatOAuthOpenEnabled"
              :wechat-mp-enabled="wechatOAuthMPEnabled"
            />
          </section>

          <section v-else-if="activeTab === 'referrals'" class="profile-settings-section">
            <h2 class="profile-settings-section-title">{{ t('profile.referralCodesTitle') }}</h2>
            <ProfileReferralCodesCard :user="user" />
          </section>

          <section v-else-if="activeTab === 'security'" class="profile-settings-section">
            <h2 class="profile-settings-section-title">{{ t('profile.navSecurity') }}</h2>
            <div class="profile-settings-stack">
              <ProfilePasswordForm />
              <ProfileTotpCard />
            </div>
          </section>

          <section
            v-else-if="activeTab === 'notifications' && user && balanceLowNotifyEnabled"
            class="profile-settings-section"
          >
            <h2 class="profile-settings-section-title">{{ t('profile.navNotifications') }}</h2>
            <ProfileBalanceNotifyCard
              :enabled="user.balance_notify_enabled ?? true"
              :threshold="user.balance_notify_threshold"
              :extra-emails="user.balance_notify_extra_emails ?? []"
              :system-default-threshold="systemDefaultThreshold"
              :user-email="user.email"
            />
          </section>

          <section v-else-if="activeTab === 'account'" class="profile-settings-section">
            <h2 class="profile-settings-section-title">{{ t('profile.navAccount') }}</h2>
            <div class="profile-settings-stack">
              <div v-if="contactInfo" class="profile-support-card">
                <div class="flex items-center gap-4">
                  <div class="profile-support-icon">
                    <Icon name="chat" size="lg" />
                  </div>
                  <div>
                    <h3 class="font-semibold text-gray-900 dark:text-white">
                      {{ t('common.contactSupport') }}
                    </h3>
                    <p class="text-sm text-gray-600 dark:text-gray-300">{{ contactInfo }}</p>
                  </div>
                </div>
              </div>

              <div class="profile-logout-card">
                <div>
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {{ t('profile.accountSession') }}
                  </h3>
                  <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('profile.logoutDescription') }}</p>
                </div>
                <button type="button" class="btn btn-danger" @click="handleLogout">
                  {{ t('nav.logout') }}
                </button>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfileBalanceNotifyCard from '@/components/user/profile/ProfileBalanceNotifyCard.vue'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'
import ProfileReferralCodesCard from '@/components/user/profile/ProfileReferralCodesCard.vue'
import ProfileTotpCard from '@/components/user/profile/ProfileTotpCard.vue'
import { isWeChatWebOAuthEnabled } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

type ProfileTab = 'profile' | 'referrals' | 'security' | 'notifications' | 'account'

function normalizeProfileTab(value: unknown): ProfileTab {
  if (value === 'referrals' || value === 'security' || value === 'notifications' || value === 'account') return value
  return 'profile'
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const user = computed(() => authStore.user)

const activeTab = ref<ProfileTab>(normalizeProfileTab(route.query.tab))

const contactInfo = ref('')
const balanceLowNotifyEnabled = ref(false)
const systemDefaultThreshold = ref(0)
const linuxdoOAuthEnabled = ref(false)
const dingtalkOAuthEnabled = ref(false)
const wechatOAuthEnabled = ref(false)
const wechatOAuthOpenEnabled = ref<boolean | undefined>(undefined)
const wechatOAuthMPEnabled = ref<boolean | undefined>(undefined)
const oidcOAuthEnabled = ref(false)
const oidcOAuthProviderName = ref('OIDC')

function selectTab(tab: ProfileTab) {
  activeTab.value = tab
  void router.replace({ path: route.path, query: { ...route.query, tab } })
}

watch(() => route.query.tab, (value) => {
  activeTab.value = normalizeProfileTab(value)
})

// The notifications tab only exists once the feature is confirmed enabled;
// fall back to the profile tab if a deep link lands here before that, or
// once settings load and reveal the feature is actually off.
watch([balanceLowNotifyEnabled, user], ([enabled, currentUser]) => {
  if (activeTab.value === 'notifications' && !(currentUser && enabled)) {
    selectTab('profile')
  }
})

onMounted(async () => {
  const profileRefresh = authStore.refreshUser().catch((error) => {
    console.error('Failed to refresh profile:', error)
  })

  const settingsLoad = appStore.fetchPublicSettings(true)
    .then((settings) => {
      if (!settings) {
        return
      }
      contactInfo.value = settings.contact_info || ''
      balanceLowNotifyEnabled.value = settings.balance_low_notify_enabled ?? false
      systemDefaultThreshold.value = settings.balance_low_notify_threshold ?? 0
      linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled ?? false
      dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
      wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
      wechatOAuthOpenEnabled.value = typeof settings.wechat_oauth_open_enabled === 'boolean'
        ? settings.wechat_oauth_open_enabled
        : undefined
      wechatOAuthMPEnabled.value = typeof settings.wechat_oauth_mp_enabled === 'boolean'
        ? settings.wechat_oauth_mp_enabled
        : undefined
      oidcOAuthEnabled.value = settings.oidc_oauth_enabled ?? false
      oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
    })
    .catch((error) => {
      console.error('Failed to load settings:', error)
    })

  await Promise.all([profileRefresh, settingsLoad])
})

async function handleLogout() {
  try {
    await authStore.logout()
  } catch (error) {
    // Ignore logout errors - still redirect to login
    console.error('Logout error:', error)
  }
  await router.push('/login')
}
</script>

<style scoped>
.profile-settings-page {
  --md-surface: #0b141c;
  --md-surface-container-low: #141c24;
  --md-surface-container: #182028;
  --md-surface-container-high: #222b33;
  --md-on-surface: #dae3ee;
  --md-on-surface-variant: #b9cbbc;
  --md-outline-variant: #3b4a3f;
  --md-primary: #00e38b;
  --md-primary-container: #1d2a31;
  --md-on-primary-container: #dae3ee;
  --md-state-hover: rgb(0 227 139 / 8%);
  --md-state-focus: rgb(0 227 139 / 18%);
  color: #dae3ee;
}

.profile-settings-header {
  max-width: 1280px;
  margin: 0 auto 40px;
}

.profile-settings-kicker {
  margin-bottom: 10px;
  color: #00e38b;
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.profile-settings-title {
  color: #f4fff8;
  font-family: 'Geist', ui-sans-serif, system-ui, sans-serif;
  font-size: 2.5rem;
  font-weight: 760;
  line-height: 1.1;
  letter-spacing: -0.01em;
}

.profile-settings-description {
  max-width: 640px;
  margin-top: 10px;
  color: #b9cbbc;
  font-size: 0.95rem;
  line-height: 1.6;
}

.profile-settings-grid {
  display: grid;
  max-width: 1280px;
  margin: 0 auto;
  gap: 40px;
}

@media (min-width: 1024px) {
  .profile-settings-grid {
    grid-template-columns: 220px minmax(0, 1fr);
    align-items: start;
  }
}

.profile-settings-nav {
  position: sticky;
  top: 32px;
  display: none;
  flex-direction: column;
  gap: 4px;
  border-left: 1px solid var(--md-outline-variant);
  padding-left: 12px;
}

@media (min-width: 1024px) {
  .profile-settings-nav {
    display: flex;
  }
}

.profile-settings-nav-link {
  border: none;
  border-radius: 6px;
  background: none;
  padding: 6px 10px;
  color: var(--md-on-surface-variant);
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
  transition: color 150ms ease, background-color 150ms ease;
}

.profile-settings-nav-link:hover {
  color: var(--md-primary);
}

.profile-settings-nav-link-active {
  background: var(--md-state-hover);
  color: var(--md-primary);
  font-weight: 700;
}

.profile-settings-nav-link-danger {
  margin-top: 8px;
  color: var(--md-error);
}

.profile-settings-nav-link-danger:hover {
  color: var(--md-error);
  opacity: 0.85;
}

.profile-settings-nav-link-danger.profile-settings-nav-link-active {
  color: var(--md-error);
  background: var(--md-state-hover);
  opacity: 1;
}

.profile-settings-content {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 48px;
}

.profile-settings-section-title {
  margin-bottom: 20px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--md-outline-variant);
  color: #f4fff8;
  font-family: 'Geist', ui-sans-serif, system-ui, sans-serif;
  font-size: 1.375rem;
  font-weight: 700;
  letter-spacing: -0.01em;
}

.profile-settings-stack {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.profile-support-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  padding: 20px;
  box-shadow: none;
  transition: border-color 0.2s ease;
}

.profile-support-card:hover {
  border-color: var(--md-primary);
}

.profile-logout-card {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  padding: 20px;
  box-shadow: none;
  transition: border-color 0.2s ease;
}

.profile-logout-card:hover {
  border-color: var(--md-error);
}

.profile-support-icon {
  display: flex;
  height: 44px;
  width: 44px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: var(--md-surface-container);
  color: var(--md-on-surface-variant);
}
</style>
