import { clearLegacyApiCache } from "@/lib/pwa-cache";

export default defineNuxtPlugin(async () => {
  await clearLegacyApiCache();
});
