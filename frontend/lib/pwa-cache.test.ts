import { describe, expect, test, vi } from "vitest";
import nuxtConfig from "@/nuxt.config";
import { clearLegacyApiCache, LEGACY_API_CACHE_NAME } from "@/lib/pwa-cache";

describe("PWA API cache remediation", () => {
  test("deletes only the legacy API cache", async () => {
    const deleteCache = vi.fn().mockResolvedValue(true);

    await expect(clearLegacyApiCache({ delete: deleteCache })).resolves.toBe(true);
    expect(deleteCache).toHaveBeenCalledOnce();
    expect(deleteCache).toHaveBeenCalledWith(LEGACY_API_CACHE_NAME);
  });

  test("does not block startup when cache deletion fails", async () => {
    const deleteCache = vi.fn().mockRejectedValue(new Error("cache unavailable"));

    await expect(clearLegacyApiCache({ delete: deleteCache })).resolves.toBe(false);
  });

  test("does not configure runtime caching for authenticated API responses", () => {
    const config = nuxtConfig as {
      pwa?: {
        workbox?: {
          navigateFallbackDenylist?: RegExp[];
          runtimeCaching?: unknown[];
        };
      };
    };

    expect(config.pwa?.workbox?.runtimeCaching).toBeUndefined();
    expect(config.pwa?.workbox?.navigateFallbackDenylist).toEqual([/^\/api/]);
  });
});
