import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { flushPromises, mount } from "@vue/test-utils";

const { getSettings, updateSettings, getOrders, getGroups, getProxies, showError, showSuccess } = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getOrders: vi.fn(),
  getGroups: vi.fn(),
  getProxies: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock("@/api/admin/settings", () => ({
  settingsAPI: {
    getAutoSupplySettings: getSettings,
    updateAutoSupplySettings: updateSettings,
    getAutoSupplyOrders: getOrders,
  },
}));
vi.mock("@/api", () => ({ adminAPI: { groups: { getAll: getGroups } } }));
vi.mock("@/api/admin/proxies", () => ({ getAll: getProxies }));
vi.mock("@/stores", () => ({ useAppStore: () => ({ showError, showSuccess }) }));
vi.mock("@/utils/apiError", () => ({ extractApiErrorMessage: () => "error" }));
vi.mock("@/utils/format", () => ({ formatDateTime: () => "formatted-date" }));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/common/GroupSelector.vue", async () => {
  const { defineComponent: defineMockComponent, h: renderMock } = await import("vue");
  return {
    default: defineMockComponent({
      props: { modelValue: { type: Array, default: () => [] } },
      emits: ["update:modelValue"],
      setup(props, { emit }) {
        return () => renderMock("button", {
          type: "button",
          class: "group-selector-stub",
          onClick: () => emit("update:modelValue", [...(props.modelValue as number[]), 9]),
        }, (props.modelValue as number[]).join(","));
      },
    }),
  };
});

import AutoSupplySettingsPanel from "../AutoSupplySettingsPanel.vue";

const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, default: false } },
  emits: ["update:modelValue"],
  setup(props, { emit }) {
    return () => h("input", {
      type: "checkbox",
      checked: props.modelValue,
      onChange: (event: Event) => emit("update:modelValue", (event.target as HTMLInputElement).checked),
    });
  },
});

const SelectStub = defineComponent({
  props: { modelValue: { type: [String, Number], default: "" }, options: { type: Array, default: () => [] } },
  emits: ["update:modelValue"],
  setup(props, { emit }) {
    return () => h("select", {
      value: props.modelValue,
      onChange: (event: Event) => emit("update:modelValue", (event.target as HTMLSelectElement).value),
    }, (props.options as Array<{ value: string | number; label: string }>).map((option) => h("option", { value: option.value }, option.label)));
  },
});

const baseSettings = {
  enabled: true,
  base_url: "https://supplier.example",
  customer_token_configured: true,
  encryption_key_configured: true,
  interval_seconds: 30,
  request_timeout_seconds: 20,
  max_quantity_per_run: 10,
  usage_forecast_enabled: true,
  usage_lookback_minutes: 360,
  usage_forecast_minutes: 120,
  usage_safety_factor: 1.25,
  usage_min_samples: 20,
  groups: [{ group_id: 7, deploy_group_ids: [8], product: "oauth_30d", min_available: 2, quantity: 3, platform: "", account_type: "", priority: 0, concurrency: 0, openai_ws_mode: "ctx_pool", proxy_mode: "none", proxy_id: null, codex_fingerprint_mode: "off", enable_account_guard: false, account_guard_interval_minutes: 30 }],
};

describe("AutoSupplySettingsPanel", () => {
  beforeEach(() => {
    getSettings.mockResolvedValue({ ...baseSettings });
    updateSettings.mockImplementation(async (payload) => ({ ...baseSettings, ...payload, customer_token_configured: true }));
    getOrders.mockResolvedValue([]);
    getGroups.mockResolvedValue([
      { id: 7, name: "OpenAI", platform: "openai", status: "active" },
      { id: 8, name: "Shared", platform: "openai", status: "active" },
      { id: 9, name: "Overflow", platform: "openai", status: "active" },
    ]);
    getProxies.mockResolvedValue([{ id: 3, name: "Primary", host: "proxy.example", port: 8080, status: "active" }]);
    showError.mockReset();
    showSuccess.mockReset();
  });

  it("loads with a blank token field while showing configured state", async () => {
    const wrapper = mount(AutoSupplySettingsPanel, { global: { stubs: { Toggle: ToggleStub, Select: SelectStub, Icon: true } } });
    await flushPromises();

    const token = wrapper.find('input[type="password"]');
    expect(token.exists()).toBe(true);
    expect((token.element as HTMLInputElement).value).toBe("");
    expect(token.attributes("placeholder")).toContain("customerTokenConfiguredPlaceholder");
    expect(wrapper.get('input[type="url"]').classes()).toContain("input");
    expect(wrapper.find(".form-input").exists()).toBe(false);
  });

  it("adds and removes rules and saves the visible values", async () => {
    const wrapper = mount(AutoSupplySettingsPanel, { global: { stubs: { Toggle: ToggleStub, Select: SelectStub, Icon: true } } });
    await flushPromises();

    const addButton = wrapper.findAll("button").find((button) => button.text().includes("addRule"));
    await addButton?.trigger("click");
    expect(wrapper.findAll('input[type="text"]').length).toBe(2);
    const removeButtons = wrapper.findAll('button[title="admin.settings.autoSupply.removeRule"]');
    await removeButtons[1]?.trigger("click");
    await wrapper.get(".group-selector-stub").trigger("click");

    const saveButton = wrapper.findAll("button").find((button) => button.text().includes("save"));
    await saveButton?.trigger("click");
    await flushPromises();

    expect(updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      enabled: true,
      base_url: "https://supplier.example",
      customer_token: undefined,
      usage_forecast_enabled: true,
      usage_lookback_minutes: 360,
      usage_forecast_minutes: 120,
      usage_safety_factor: 1.25,
      usage_min_samples: 20,
      groups: [expect.objectContaining({ group_id: 7, deploy_group_ids: [8, 9] })],
    }));
    expect(showSuccess).toHaveBeenCalled();
  });

  it("shows usage forecast controls", async () => {
    const wrapper = mount(AutoSupplySettingsPanel, { global: { stubs: { Toggle: ToggleStub, Select: SelectStub, Icon: true } } });
    await flushPromises();

    expect(wrapper.text()).toContain("admin.settings.autoSupply.usageForecast");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.usageLookbackMinutes");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.usageForecastMinutes");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.usageSafetyFactor");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.usageMinSamples");
  });

  it("shows proxy, OAuth convergence, and account guard controls for a rule", async () => {
    getSettings.mockResolvedValue({
      ...baseSettings,
      groups: [{ ...baseSettings.groups[0], proxy_mode: "specified", proxy_id: 3, codex_fingerprint_mode: "session", enable_account_guard: true, account_guard_interval_minutes: 45 }],
    });
    const wrapper = mount(AutoSupplySettingsPanel, { global: { stubs: { Toggle: ToggleStub, Select: SelectStub, Icon: true } } });
    await flushPromises();

    expect(wrapper.text()).toContain("admin.settings.autoSupply.proxyMode");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.oauthConvergence");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.openAIWSMode");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.openAIWSModeCtxPool");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.enableAccountGuard");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.accountGuardInterval");
    expect(wrapper.text()).toContain("Primary (proxy.example:8080)");
  });

  it("renders replenishment order history", async () => {
    getOrders.mockResolvedValue([{
      id: "order-1",
      group_id: 7,
      product: "oauth_30d",
      quantity: 1,
      status: "completed",
      created_at: "2026-08-18T00:00:00Z",
      updated_at: "2026-08-18T00:01:00Z",
    }]);
    const wrapper = mount(AutoSupplySettingsPanel, { global: { stubs: { Toggle: ToggleStub, Select: SelectStub, Icon: true } } });
    await flushPromises();

    expect(wrapper.text()).toContain("order-1");
    expect(wrapper.text()).toContain("oauth_30d");
    expect(wrapper.text()).toContain("admin.settings.autoSupply.orderStatus_completed");
    expect(wrapper.text()).toContain("OpenAI (#7)");
  });
});
