export const LEGACY_API_CACHE_NAME = "api-cache";

type CacheStorageDelete = Pick<CacheStorage, "delete">;

/**
 * Remove the legacy API cache, whose entries were not partitioned by the
 * authenticated user or collection.
 */
export async function clearLegacyApiCache(cacheStorage?: CacheStorageDelete): Promise<boolean> {
  const storage = cacheStorage ?? (typeof caches === "undefined" ? undefined : caches);
  if (!storage) {
    return false;
  }

  try {
    return await storage.delete(LEGACY_API_CACHE_NAME);
  } catch {
    // Cache cleanup must not prevent the application from starting.
    return false;
  }
}
