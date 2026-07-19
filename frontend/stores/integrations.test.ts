import { beforeEach, describe, expect, test, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { ref } from "vue";
import { useIntegrationsStore } from "./integrations";
import type { GroupIntegrationsOut } from "~~/lib/api/types/data-contracts";

const preferences = ref({ collectionId: "collection-a" as string | null });

function integrationsData(aiConfigured: boolean): GroupIntegrationsOut {
  return {
    aiApiKey: "",
    aiBaseUrl: "",
    aiConfigured,
    aiModel: "",
    aiProvider: "",
    barcodeTokenBarcodespider: "",
    barcodespiderConfigured: false,
    envAiBaseUrl: "",
    envAiModel: "",
    envAiProvider: "",
    isOwner: true,
    openFoodFactsContact: "",
  };
}

beforeEach(() => {
  setActivePinia(createPinia());
  preferences.value.collectionId = "collection-a";
  vi.stubGlobal("useViewPreferences", () => preferences);
});

describe("useIntegrationsStore", () => {
  test("does not expose one collection's config after the selection changes", async () => {
    const integrations = vi.fn().mockResolvedValue({
      error: false,
      data: integrationsData(true),
    });
    vi.stubGlobal("useUserApi", () => ({ group: { integrations } }));

    const store = useIntegrationsStore();
    await store.refresh();
    expect(store.currentCollectionLoaded).toBe(true);
    expect(store.aiConfigured).toBe(true);

    preferences.value.collectionId = "collection-b";
    expect(store.currentCollectionLoaded).toBe(false);
    expect(store.aiConfigured).toBe(false);
  });

  test("ignores a stale response that finishes after a newer collection refresh", async () => {
    let resolveA!: (value: unknown) => void;
    let resolveB!: (value: unknown) => void;
    const requestA = new Promise(resolve => {
      resolveA = resolve;
    });
    const requestB = new Promise(resolve => {
      resolveB = resolve;
    });
    const integrations = vi.fn().mockReturnValueOnce(requestA).mockReturnValueOnce(requestB);
    vi.stubGlobal("useUserApi", () => ({ group: { integrations } }));

    const store = useIntegrationsStore();
    const refreshA = store.refresh();
    preferences.value.collectionId = "collection-b";
    const refreshB = store.refresh();

    resolveB({ error: false, data: integrationsData(false) });
    await refreshB;
    resolveA({ error: false, data: integrationsData(true) });
    await refreshA;

    expect(store.dataCollectionId).toBe("collection-b");
    expect(store.data?.aiConfigured).toBe(false);
    expect(store.currentCollectionLoaded).toBe(true);
    expect(store.aiConfigured).toBe(false);
  });

  test("deduplicates ownership and feature requests for the selected collection", async () => {
    let resolveRequest!: (value: unknown) => void;
    const request = new Promise(resolve => {
      resolveRequest = resolve;
    });
    const integrations = vi.fn().mockReturnValue(request);
    vi.stubGlobal("useUserApi", () => ({ group: { integrations } }));

    const store = useIntegrationsStore();
    const first = store.ensureFetched();
    const second = store.ensureFetched();

    expect(integrations).toHaveBeenCalledTimes(1);
    resolveRequest({ error: false, data: integrationsData(true) });
    await Promise.all([first, second]);

    expect(store.currentCollectionLoaded).toBe(true);
    expect(store.isOwner).toBe(true);
  });

  test("does not request collection settings while the user is groupless", async () => {
    const integrations = vi.fn();
    vi.stubGlobal("useUserApi", () => ({ group: { integrations } }));
    preferences.value.collectionId = null;

    const store = useIntegrationsStore();
    await store.ensureFetched();

    expect(integrations).not.toHaveBeenCalled();
    expect(store.currentCollectionLoaded).toBe(false);
    expect(store.isOwner).toBe(false);
  });
});
