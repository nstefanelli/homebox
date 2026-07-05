import { defineStore } from "pinia";
import type { GroupIntegrationsOut } from "~~/lib/api/types/data-contracts";

export const useIntegrationsStore = defineStore("integrations", {
  state: () => ({
    data: null as GroupIntegrationsOut | null,
    client: useUserApi(),
    refreshPromise: null as Promise<void> | null,
  }),
  getters: {
    aiConfigured(state): boolean {
      return state.data?.aiConfigured ?? false;
    },
    barcodespiderConfigured(state): boolean {
      return state.data?.barcodespiderConfigured ?? false;
    },
    isOwner(state): boolean {
      return state.data?.isOwner ?? false;
    },
  },
  actions: {
    async ensureFetched() {
      if (this.data !== null) return;

      if (this.refreshPromise === null) {
        this.refreshPromise = this.refresh();
      }

      await this.refreshPromise;
    },

    async refresh() {
      if (this.refreshPromise !== null) return this.refreshPromise;

      this.refreshPromise = (async () => {
        const result = await this.client.group.integrations();
        if (result.error) throw result.error;
        this.data = result.data ?? null;
      })();

      try {
        await this.refreshPromise;
      } finally {
        this.refreshPromise = null;
      }
    },
  },
});

export default useIntegrationsStore;
