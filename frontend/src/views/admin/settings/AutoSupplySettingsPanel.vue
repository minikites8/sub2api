<template>
  <div class="space-y-6">
    <section class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t("admin.settings.autoSupply.title") }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.autoSupply.description") }}
            </p>
          </div>
          <label class="flex shrink-0 items-center gap-2 whitespace-nowrap text-sm font-medium text-gray-700 dark:text-gray-300">
            <Toggle v-model="form.enabled" />
            <span>{{ t("admin.settings.autoSupply.enabled") }}</span>
          </label>
        </div>
      </div>

      <div class="space-y-5 p-6">
        <div
          v-if="!form.encryption_key_configured"
          class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
        >
          {{ t("admin.settings.autoSupply.encryptionKeyHint") }}
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <label class="block md:col-span-2">
            <span class="input-label">{{ t("admin.settings.autoSupply.baseUrl") }}</span>
            <input
              v-model.trim="form.base_url"
              type="url"
              class="input mt-1 w-full"
              :placeholder="t('admin.settings.autoSupply.baseUrlPlaceholder')"
              autocomplete="url"
            />
          </label>

          <label class="block md:col-span-2">
            <span class="input-label">{{ t("admin.settings.autoSupply.customerToken") }}</span>
            <input
              v-model="form.customer_token"
              type="password"
              class="input mt-1 w-full"
              :placeholder="form.customer_token_configured
                ? t('admin.settings.autoSupply.customerTokenConfiguredPlaceholder')
                : t('admin.settings.autoSupply.customerTokenPlaceholder')"
              autocomplete="new-password"
            />
            <span class="input-hint mt-1 block">
              {{ form.customer_token_configured
                ? t("admin.settings.autoSupply.customerTokenConfiguredHint")
                : t("admin.settings.autoSupply.customerTokenHint") }}
            </span>
          </label>

          <label class="block">
            <span class="input-label">{{ t("admin.settings.autoSupply.interval") }}</span>
            <input v-model.number="form.interval_seconds" type="number" min="5" max="86400" class="input mt-1 w-full" />
            <span class="input-hint mt-1 block">{{ t("admin.settings.autoSupply.intervalHint") }}</span>
          </label>

          <label class="block">
            <span class="input-label">{{ t("admin.settings.autoSupply.timeout") }}</span>
            <input v-model.number="form.request_timeout_seconds" type="number" min="1" max="300" class="input mt-1 w-full" />
            <span class="input-hint mt-1 block">{{ t("admin.settings.autoSupply.timeoutHint") }}</span>
          </label>

          <label class="block">
            <span class="input-label">{{ t("admin.settings.autoSupply.maxQuantity") }}</span>
            <input v-model.number="form.max_quantity_per_run" type="number" min="1" max="1000" class="input mt-1 w-full" />
            <span class="input-hint mt-1 block">{{ t("admin.settings.autoSupply.maxQuantityHint") }}</span>
          </label>
        </div>
      </div>
    </section>

    <section class="card">
      <div class="flex items-center justify-between gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t("admin.settings.autoSupply.rulesTitle") }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.autoSupply.rulesDescription") }}
          </p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" @click="addRule">
          <Icon name="plus" size="sm" />
          <span>{{ t("admin.settings.autoSupply.addRule") }}</span>
        </button>
      </div>

      <div class="space-y-4 p-6">
        <div v-if="form.groups.length === 0" class="rounded border border-dashed border-gray-300 px-4 py-5 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t("admin.settings.autoSupply.noRules") }}
        </div>

        <div v-for="(rule, index) in form.groups" :key="ruleKey(rule, index)" class="rounded border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between gap-3">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.autoSupply.ruleNumber", { index: index + 1 }) }}
            </span>
            <button type="button" class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" :aria-label="t('admin.settings.autoSupply.removeRule')" :title="t('admin.settings.autoSupply.removeRule')" @click="removeRule(index)">
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <label class="block sm:col-span-2 lg:col-span-2">
              <span class="input-label">{{ t("admin.settings.autoSupply.group") }}</span>
              <Select v-model="rule.group_id" class="mt-1" :options="groupOptions" :placeholder="t('admin.settings.autoSupply.groupPlaceholder')" searchable="auto" @change="handleTriggerGroupChange(rule)" />
            </label>
            <div class="sm:col-span-2 lg:col-span-2">
              <GroupSelector
                v-model="rule.deploy_group_ids"
                :groups="deployableGroups(rule)"
                :label="t('admin.settings.autoSupply.deployGroups')"
                searchable="auto"
              />
              <p class="input-hint">{{ t("admin.settings.autoSupply.deployGroupsHint") }}</p>
            </div>
            <label class="block sm:col-span-2 lg:col-span-2">
              <span class="input-label">{{ t("admin.settings.autoSupply.product") }}</span>
              <input v-model.trim="rule.product" type="text" class="input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.minAvailable") }}</span>
              <input v-model.number="rule.min_available" type="number" min="0" class="input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.quantity") }}</span>
              <input v-model.number="rule.quantity" type="number" min="0" class="input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.platform") }}</span>
              <Select v-model="rule.platform" class="mt-1" :options="platformOptions" :placeholder="t('admin.settings.autoSupply.inheritGroupValue')" />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.accountType") }}</span>
              <Select v-model="rule.account_type" class="mt-1" :options="accountTypeOptions" :placeholder="t('admin.settings.autoSupply.defaultAccountType')" />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.priority") }}</span>
              <input v-model.number="rule.priority" type="number" min="0" class="input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.concurrency") }}</span>
              <input v-model.number="rule.concurrency" type="number" min="0" class="input mt-1 w-full" />
              <span class="input-hint mt-1 block">{{ t("admin.settings.autoSupply.concurrencyHint") }}</span>
            </label>
            <label v-if="isOpenAIRule(rule)" class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.openAIWSMode") }}</span>
              <Select v-model="rule.openai_ws_mode" class="mt-1" :options="openAIWSModeOptions" />
              <span class="input-hint mt-1 block">{{ t("admin.settings.autoSupply.openAIWSModeHint") }}</span>
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.proxyMode") }}</span>
              <Select v-model="rule.proxy_mode" class="mt-1" :options="proxyModeOptions" />
            </label>
            <label v-if="rule.proxy_mode === 'specified'" class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.proxy") }}</span>
              <Select
                v-model="rule.proxy_id"
                class="mt-1"
                :options="proxyOptions"
                :placeholder="t('admin.settings.autoSupply.proxyPlaceholder')"
                searchable="auto"
              />
            </label>
            <label class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.oauthConvergence") }}</span>
              <Select v-model="rule.codex_fingerprint_mode" class="mt-1" :options="fingerprintOptions" />
            </label>
            <div class="flex items-end">
              <label class="flex items-center gap-2 pb-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="rule.enable_account_guard" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                {{ t("admin.settings.autoSupply.enableAccountGuard") }}
              </label>
            </div>
            <label v-if="rule.enable_account_guard" class="block">
              <span class="input-label">{{ t("admin.settings.autoSupply.accountGuardInterval") }}</span>
              <input v-model.number="rule.account_guard_interval_minutes" type="number" min="5" max="1440" class="input mt-1 w-full" />
              <span class="input-hint mt-1 block">{{ t("admin.settings.autoSupply.accountGuardIntervalHint") }}</span>
            </label>
          </div>
        </div>

        <p v-if="validationError" class="text-sm text-red-600 dark:text-red-400">{{ validationError }}</p>
        <div class="flex justify-end">
          <button type="button" class="btn btn-primary" :disabled="saving || loading" @click="saveSettings">
            <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="check" size="sm" />
            <span>{{ saving ? t("admin.settings.autoSupply.saving") : t("admin.settings.autoSupply.save") }}</span>
          </button>
        </div>
      </div>
    </section>

    <section class="card">
      <div class="flex items-center justify-between gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t("admin.settings.autoSupply.ordersTitle") }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.autoSupply.ordersDescription") }}
          </p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="ordersLoading" @click="loadOrders">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': ordersLoading }" />
          <span>{{ t("admin.settings.autoSupply.refreshOrders") }}</span>
        </button>
      </div>

      <div class="p-6">
        <div v-if="ordersLoading && orders.length === 0" class="flex items-center gap-2 py-5 text-sm text-gray-500 dark:text-gray-400">
          <Icon name="refresh" size="sm" class="animate-spin" />
          {{ t("admin.settings.autoSupply.loadingOrders") }}
        </div>
        <div v-else-if="orders.length === 0" class="rounded border border-dashed border-gray-300 px-4 py-5 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t("admin.settings.autoSupply.noOrders") }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="border-b border-gray-200 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
              <tr>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderId") }}</th>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderGroup") }}</th>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderProduct") }}</th>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderQuantity") }}</th>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderStatus") }}</th>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderCreatedAt") }}</th>
                <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderUpdatedAt") }}</th>
                <th class="px-3 py-2 font-medium">{{ t("admin.settings.autoSupply.orderError") }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="order in orders" :key="order.id" class="align-top">
                <td class="max-w-48 px-3 py-3 font-mono text-xs text-gray-700 dark:text-gray-300">
                  <span class="block truncate" :title="order.id">{{ order.id }}</span>
                </td>
                <td class="whitespace-nowrap px-3 py-3 text-gray-700 dark:text-gray-300">{{ orderGroupLabel(order.group_id) }}</td>
                <td class="whitespace-nowrap px-3 py-3 text-gray-700 dark:text-gray-300">{{ order.product }}</td>
                <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ order.quantity }}</td>
                <td class="px-3 py-3">
                  <span class="inline-flex whitespace-nowrap rounded px-2 py-0.5 text-xs font-medium" :class="orderStatusClass(order.status)">
                    {{ orderStatusLabel(order.status) }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-3 py-3 text-xs text-gray-500 dark:text-gray-400">{{ formatOrderTime(order.created_at) }}</td>
                <td class="whitespace-nowrap px-3 py-3 text-xs text-gray-500 dark:text-gray-400">{{ formatOrderTime(order.updated_at) }}</td>
                <td class="max-w-72 px-3 py-3 text-xs text-red-600 dark:text-red-400">
                  <span class="block truncate" :title="order.error || ''">{{ order.error || "-" }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import { settingsAPI, type AutoSupplyGroupSettings, type AutoSupplyOrder, type AutoSupplySettings } from "@/api/admin/settings";
import { getAll as getAllProxies } from "@/api/admin/proxies";
import type { AdminGroup } from "@/types";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";
import GroupSelector from "@/components/common/GroupSelector.vue";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";
import { formatDateTime } from "@/utils/format";

const { t } = useI18n();
const appStore = useAppStore();
const loading = ref(true);
const saving = ref(false);
const ordersLoading = ref(false);
const validationError = ref("");
const groups = ref<AdminGroup[]>([]);
const orders = ref<AutoSupplyOrder[]>([]);
const proxies = ref<Array<{ id: number; name: string; host: string; port: number }>>([]);
type FormState = AutoSupplySettings & { customer_token: string };
const form = reactive<FormState>({
  enabled: false,
  base_url: "",
  customer_token_configured: false,
  encryption_key_configured: false,
  customer_token: "",
  interval_seconds: 30,
  request_timeout_seconds: 20,
  max_quantity_per_run: 10,
  groups: [],
});

const platformOptions = computed(() => [
  { value: "", label: t("admin.settings.autoSupply.inheritGroupValue") },
  ...["openai", "anthropic", "gemini", "antigravity", "kiro", "grok"].map((value) => ({ value, label: value })),
]);
const proxyModeOptions = computed(() => [
  { value: "none", label: t("admin.settings.autoSupply.proxyNone") },
  { value: "specified", label: t("admin.settings.autoSupply.proxySpecified") },
  { value: "random", label: t("admin.settings.autoSupply.proxyRandom") },
]);
const fingerprintOptions = computed(() => [
  { value: "off", label: t("admin.settings.autoSupply.oauthConvergenceOff") },
  { value: "device", label: t("admin.settings.autoSupply.oauthConvergenceDevice") },
  { value: "session", label: t("admin.settings.autoSupply.oauthConvergenceSession") },
  { value: "full", label: t("admin.settings.autoSupply.oauthConvergenceFull") },
]);
const openAIWSModeOptions = computed(() => [
  { value: "off", label: t("admin.settings.autoSupply.openAIWSModeOff") },
  { value: "ctx_pool", label: t("admin.settings.autoSupply.openAIWSModeCtxPool") },
  { value: "passthrough", label: t("admin.settings.autoSupply.openAIWSModePassthrough") },
  { value: "http_bridge", label: t("admin.settings.autoSupply.openAIWSModeHTTPBridge") },
]);
const proxyOptions = computed(() => proxies.value.map((proxy) => ({
  value: proxy.id,
  label: `${proxy.name} (${proxy.host}:${proxy.port})`,
})));
const accountTypeOptions = computed(() => [
  { value: "", label: t("admin.settings.autoSupply.defaultAccountType") },
  { value: "oauth", label: "OAuth" },
  { value: "setup-token", label: "Setup Token" },
  { value: "apikey", label: "API Key" },
  { value: "upstream", label: t("admin.settings.autoSupply.accountTypeUpstream") },
]);
const groupOptions = computed(() => {
  const known = new Set(groups.value.map((group) => group.id));
  const selected = form.groups.map((rule) => rule.group_id).filter((id) => id > 0 && !known.has(id));
  return [
    ...groups.value.map((group) => ({ value: group.id, label: `${group.name} (#${group.id})` })),
    ...selected.map((id) => ({ value: id, label: `#${id} (${t("admin.settings.autoSupply.groupUnavailable")})` })),
  ];
});

function ruleKey(rule: AutoSupplyGroupSettings, index: number): string { return `${index}-${rule.group_id}`; }
function emptyRule(groupId = 0): AutoSupplyGroupSettings {
  return {
    group_id: groupId,
    deploy_group_ids: [],
    product: "oauth_30d",
    min_available: 1,
    quantity: 1,
    platform: "",
    account_type: "",
    priority: 0,
    concurrency: 0,
    openai_ws_mode: "off",
    proxy_mode: "none",
    proxy_id: null,
    codex_fingerprint_mode: "off",
    enable_account_guard: false,
    account_guard_interval_minutes: 30,
  };
}
function addRule(): void {
  const used = new Set(form.groups.map((rule) => rule.group_id));
  const first = groups.value.find((group) => !used.has(group.id));
  form.groups.push(emptyRule(first?.id ?? 0));
}
function removeRule(index: number): void { form.groups.splice(index, 1); }
function deployableGroups(rule: AutoSupplyGroupSettings): AdminGroup[] {
  return groups.value.filter((group) => group.id !== rule.group_id);
}
function handleTriggerGroupChange(rule: AutoSupplyGroupSettings): void {
  rule.deploy_group_ids = rule.deploy_group_ids.filter((groupId) => groupId !== rule.group_id);
}
function isOpenAIRule(rule: AutoSupplyGroupSettings): boolean {
  const platform = rule.platform || groups.value.find((group) => group.id === rule.group_id)?.platform || "";
  return platform.toLowerCase() === "openai";
}
function applySettings(settings: AutoSupplySettings): void {
  Object.assign(form, {
    ...settings,
    groups: settings.groups.map((rule) => ({
      ...emptyRule(rule.group_id),
      ...rule,
      deploy_group_ids: [...(rule.deploy_group_ids ?? [])],
      proxy_id: rule.proxy_id ?? null,
    })),
    customer_token: "",
  });
}
function validate(): boolean {
  validationError.value = "";
  if (form.enabled && !form.base_url.trim()) { validationError.value = t("admin.settings.autoSupply.baseUrlRequired"); return false; }
  if (form.interval_seconds < 5 || form.interval_seconds > 86400) { validationError.value = t("admin.settings.autoSupply.intervalInvalid"); return false; }
  if (form.request_timeout_seconds < 1 || form.request_timeout_seconds > 300) { validationError.value = t("admin.settings.autoSupply.timeoutInvalid"); return false; }
  if (form.max_quantity_per_run < 1 || form.max_quantity_per_run > 1000) { validationError.value = t("admin.settings.autoSupply.maxQuantityInvalid"); return false; }
  if (form.enabled && form.groups.length === 0) { validationError.value = t("admin.settings.autoSupply.rulesRequired"); return false; }
  if (form.groups.some((rule) => rule.group_id <= 0 || !rule.product.trim() || rule.deploy_group_ids.some((id) => id <= 0))) { validationError.value = t("admin.settings.autoSupply.ruleInvalid"); return false; }
  if (form.groups.some((rule) => rule.proxy_mode === "specified" && !rule.proxy_id)) { validationError.value = t("admin.settings.autoSupply.proxyRequired"); return false; }
  if (form.groups.some((rule) => rule.enable_account_guard && (!Number.isInteger(rule.account_guard_interval_minutes) || rule.account_guard_interval_minutes < 5 || rule.account_guard_interval_minutes > 1440))) { validationError.value = t("admin.settings.autoSupply.accountGuardIntervalInvalid"); return false; }
  return true;
}
async function saveSettings(): Promise<void> {
  if (!validate()) return;
  saving.value = true;
  try {
    const saved = await settingsAPI.updateAutoSupplySettings({
      enabled: form.enabled, base_url: form.base_url.trim(), customer_token: form.customer_token.trim() || undefined,
      interval_seconds: form.interval_seconds, request_timeout_seconds: form.request_timeout_seconds,
      max_quantity_per_run: form.max_quantity_per_run, groups: form.groups.map((rule) => ({ ...rule })),
    });
    applySettings(saved);
    appStore.showSuccess(t("admin.settings.autoSupply.saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.autoSupply.saveFailed")));
  } finally { saving.value = false; }
}
async function load(): Promise<void> {
  loading.value = true;
  try {
    const [settings, allGroups] = await Promise.all([settingsAPI.getAutoSupplySettings(), adminAPI.groups.getAll()]);
    groups.value = allGroups.filter((group) => group.status === "active");
    try {
      proxies.value = (await getAllProxies()).filter((proxy) => proxy.status === "active");
    } catch {
      proxies.value = [];
    }
    applySettings(settings);
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.autoSupply.loadFailed")));
  } finally { loading.value = false; }
}
async function loadOrders(): Promise<void> {
  ordersLoading.value = true;
  try {
    orders.value = await settingsAPI.getAutoSupplyOrders();
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.autoSupply.ordersLoadFailed")));
  } finally {
    ordersLoading.value = false;
  }
}
function orderGroupLabel(groupId: number): string {
  const group = groups.value.find((item) => item.id === groupId);
  return group ? `${group.name} (#${groupId})` : `#${groupId}`;
}
function orderStatusLabel(status: string): string {
  const normalized = status.trim().toLowerCase();
  const known = ["pending", "queued", "processing", "importing", "completed", "failed", "import_failed", "cancelled", "expired", "rejected"];
  return known.includes(normalized) ? t(`admin.settings.autoSupply.orderStatus_${normalized}`) : status;
}
function orderStatusClass(status: string): string {
  switch (status.trim().toLowerCase()) {
    case "completed":
      return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300";
    case "failed":
    case "import_failed":
    case "rejected":
      return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
    case "expired":
    case "cancelled":
      return "bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300";
    default:
      return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300";
  }
}
function formatOrderTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : formatDateTime(date);
}
onMounted(() => {
  void load();
  void loadOrders();
});
</script>
