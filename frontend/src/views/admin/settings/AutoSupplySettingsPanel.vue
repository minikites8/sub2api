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
          <label class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
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
            <span class="form-label">{{ t("admin.settings.autoSupply.baseUrl") }}</span>
            <input
              v-model.trim="form.base_url"
              type="url"
              class="form-input mt-1 w-full"
              :placeholder="t('admin.settings.autoSupply.baseUrlPlaceholder')"
              autocomplete="url"
            />
          </label>

          <label class="block md:col-span-2">
            <span class="form-label">{{ t("admin.settings.autoSupply.customerToken") }}</span>
            <input
              v-model="form.customer_token"
              type="password"
              class="form-input mt-1 w-full"
              :placeholder="form.customer_token_configured
                ? t('admin.settings.autoSupply.customerTokenConfiguredPlaceholder')
                : t('admin.settings.autoSupply.customerTokenPlaceholder')"
              autocomplete="new-password"
            />
            <span class="form-hint mt-1 block">
              {{ form.customer_token_configured
                ? t("admin.settings.autoSupply.customerTokenConfiguredHint")
                : t("admin.settings.autoSupply.customerTokenHint") }}
            </span>
          </label>

          <label class="block">
            <span class="form-label">{{ t("admin.settings.autoSupply.interval") }}</span>
            <input v-model.number="form.interval_seconds" type="number" min="5" max="86400" class="form-input mt-1 w-full" />
            <span class="form-hint mt-1 block">{{ t("admin.settings.autoSupply.intervalHint") }}</span>
          </label>

          <label class="block">
            <span class="form-label">{{ t("admin.settings.autoSupply.timeout") }}</span>
            <input v-model.number="form.request_timeout_seconds" type="number" min="1" max="300" class="form-input mt-1 w-full" />
            <span class="form-hint mt-1 block">{{ t("admin.settings.autoSupply.timeoutHint") }}</span>
          </label>

          <label class="block">
            <span class="form-label">{{ t("admin.settings.autoSupply.maxQuantity") }}</span>
            <input v-model.number="form.max_quantity_per_run" type="number" min="1" max="1000" class="form-input mt-1 w-full" />
            <span class="form-hint mt-1 block">{{ t("admin.settings.autoSupply.maxQuantityHint") }}</span>
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
              <span class="form-label">{{ t("admin.settings.autoSupply.group") }}</span>
              <Select v-model="rule.group_id" class="mt-1" :options="groupOptions" :placeholder="t('admin.settings.autoSupply.groupPlaceholder')" searchable="auto" />
            </label>
            <label class="block sm:col-span-2 lg:col-span-2">
              <span class="form-label">{{ t("admin.settings.autoSupply.product") }}</span>
              <input v-model.trim="rule.product" type="text" class="form-input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="form-label">{{ t("admin.settings.autoSupply.minAvailable") }}</span>
              <input v-model.number="rule.min_available" type="number" min="0" class="form-input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="form-label">{{ t("admin.settings.autoSupply.quantity") }}</span>
              <input v-model.number="rule.quantity" type="number" min="0" class="form-input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="form-label">{{ t("admin.settings.autoSupply.platform") }}</span>
              <Select v-model="rule.platform" class="mt-1" :options="platformOptions" :placeholder="t('admin.settings.autoSupply.inheritGroupValue')" />
            </label>
            <label class="block">
              <span class="form-label">{{ t("admin.settings.autoSupply.accountType") }}</span>
              <Select v-model="rule.account_type" class="mt-1" :options="accountTypeOptions" :placeholder="t('admin.settings.autoSupply.defaultAccountType')" />
            </label>
            <label class="block">
              <span class="form-label">{{ t("admin.settings.autoSupply.priority") }}</span>
              <input v-model.number="rule.priority" type="number" min="0" class="form-input mt-1 w-full" />
            </label>
            <label class="block">
              <span class="form-label">{{ t("admin.settings.autoSupply.concurrency") }}</span>
              <input v-model.number="rule.concurrency" type="number" min="0" class="form-input mt-1 w-full" />
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import { settingsAPI, type AutoSupplyGroupSettings, type AutoSupplySettings } from "@/api/admin/settings";
import type { AdminGroup } from "@/types";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";

const { t } = useI18n();
const appStore = useAppStore();
const loading = ref(true);
const saving = ref(false);
const validationError = ref("");
const groups = ref<AdminGroup[]>([]);
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
  return { group_id: groupId, product: "oauth_30d", min_available: 1, quantity: 1, platform: "", account_type: "", priority: 0, concurrency: 0 };
}
function addRule(): void {
  const used = new Set(form.groups.map((rule) => rule.group_id));
  const first = groups.value.find((group) => !used.has(group.id));
  form.groups.push(emptyRule(first?.id ?? 0));
}
function removeRule(index: number): void { form.groups.splice(index, 1); }
function applySettings(settings: AutoSupplySettings): void {
  Object.assign(form, { ...settings, groups: settings.groups.map((rule) => ({ ...rule })), customer_token: "" });
}
function validate(): boolean {
  validationError.value = "";
  if (form.enabled && !form.base_url.trim()) { validationError.value = t("admin.settings.autoSupply.baseUrlRequired"); return false; }
  if (form.interval_seconds < 5 || form.interval_seconds > 86400) { validationError.value = t("admin.settings.autoSupply.intervalInvalid"); return false; }
  if (form.request_timeout_seconds < 1 || form.request_timeout_seconds > 300) { validationError.value = t("admin.settings.autoSupply.timeoutInvalid"); return false; }
  if (form.max_quantity_per_run < 1 || form.max_quantity_per_run > 1000) { validationError.value = t("admin.settings.autoSupply.maxQuantityInvalid"); return false; }
  if (form.enabled && form.groups.length === 0) { validationError.value = t("admin.settings.autoSupply.rulesRequired"); return false; }
  if (form.groups.some((rule) => rule.group_id <= 0 || !rule.product.trim())) { validationError.value = t("admin.settings.autoSupply.ruleInvalid"); return false; }
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
    applySettings(settings);
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.autoSupply.loadFailed")));
  } finally { loading.value = false; }
}
onMounted(load);
</script>
