import { beforeEach, describe, expect, it, vi } from "vitest";

const { get, put } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  apiClient: { get, put },
}));

import { settingsAPI } from "@/api/admin/settings";

describe("automatic supply settings API", () => {
  beforeEach(() => {
    get.mockReset();
    put.mockReset();
  });

  it("uses the dedicated admin endpoint for reads", async () => {
    get.mockResolvedValue({ data: { enabled: false, groups: [] } });

    await settingsAPI.getAutoSupplySettings();

    expect(get).toHaveBeenCalledWith("/admin/settings/auto-supply");
  });

  it("sends the token only when the admin entered one", async () => {
    const payload = {
      enabled: true,
      base_url: "https://supplier.example",
      customer_token: "secret",
      interval_seconds: 30,
      request_timeout_seconds: 20,
      max_quantity_per_run: 10,
      usage_forecast_enabled: false,
      usage_lookback_hours: 6,
      usage_forecast_hours: 2,
      usage_safety_factor: 1.25,
      usage_min_samples: 20,
      groups: [],
    };
    put.mockResolvedValue({ data: { enabled: true, customer_token_configured: true } });

    await settingsAPI.updateAutoSupplySettings(payload);

    expect(put).toHaveBeenCalledWith("/admin/settings/auto-supply", payload);
  });

  it("uses the dedicated admin endpoint for order history", async () => {
    get.mockResolvedValue({ data: [] });

    await settingsAPI.getAutoSupplyOrders();

    expect(get).toHaveBeenCalledWith("/admin/settings/auto-supply/orders");
  });
});
