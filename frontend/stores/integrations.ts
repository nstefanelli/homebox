import { defineStore } from "pinia";
import type { GroupIntegrationsOut } from "~~/lib/api/types/data-contracts";

function selectedCollectionId(): string | null {
  return useViewPreferences().value.collectionId ?? null;
}

export const useIntegrationsStore = defineStore("integrations", {
  state: () => ({
    data: null as GroupIntegrationsOut | null,
    dataCollectionId: null as string | null,
    refreshPromise: null as Promise<void> | null,
    refreshCollectionId: null as string | null,
    refreshSequence: 0,
  }),
  getters: {
    currentCollectionLoaded(state): boolean {
      return state.dataCollectionId === selectedCollectionId() && state.data !== null;
    },
    aiConfigured(state): boolean {
      return state.dataCollectionId === selectedCollectionId() && (state.data?.aiConfigured ?? false);
    },
    barcodespiderConfigured(state): boolean {
      return state.dataCollectionId === selectedCollectionId() && (state.data?.barcodespiderConfigured ?? false);
    },
    isOwner(state): boolean {
      return state.dataCollectionId === selectedCollectionId() && (state.data?.isOwner ?? false);
    },
  },
  actions: {
    async ensureFetched() {
      const collectionId = selectedCollectionId();
      if (collectionId === null) {
        this.data = null;
        this.dataCollectionId = null;
        return;
      }
      if (this.data !== null && this.dataCollectionId === collectionId) return;

      if (this.refreshPromise !== null && this.refreshCollectionId === collectionId) {
        await this.refreshPromise;
        return;
      }

      await this.refresh();
    },

    async refresh() {
      const collectionId = selectedCollectionId();
      if (collectionId === null) {
        this.data = null;
        this.dataCollectionId = null;
        return;
      }
      if (this.refreshPromise !== null && this.refreshCollectionId === collectionId) {
        return this.refreshPromise;
      }

      // A collection switch can occur while the previous request is still in
      // flight. Give each refresh a generation and only publish the newest
      // response for the still-selected collection.
      const sequence = ++this.refreshSequence;
      this.refreshCollectionId = collectionId;
      if (this.dataCollectionId !== collectionId) {
        this.data = null;
      }

      const promise = (async () => {
        const result = await useUserApi().group.integrations();
        if (result.error) throw result.error;
        if (this.refreshSequence === sequence && selectedCollectionId() === collectionId) {
          this.data = result.data ?? null;
          this.dataCollectionId = collectionId;
        }
      })();
      this.refreshPromise = promise;

      try {
        await promise;
      } finally {
        if (this.refreshSequence === sequence) {
          this.refreshPromise = null;
          this.refreshCollectionId = null;
        }
      }
    },
  },
});

export default useIntegrationsStore;
