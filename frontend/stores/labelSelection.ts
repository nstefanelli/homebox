import { defineStore } from "pinia";
import type { PrintQueueEntry } from "./labels";

export const useLabelSelection = defineStore("label-selection", {
  state: () => ({
    selectMode: false,
    selected: {} as Record<string, PrintQueueEntry>,
  }),
  getters: {
    count: state => Object.keys(state.selected).length,
  },
  actions: {
    toggle(entry: PrintQueueEntry) {
      if (this.selected[entry.id]) {
        Reflect.deleteProperty(this.selected, entry.id);
      } else {
        this.selected[entry.id] = entry;
      }
    },
    clear() {
      this.selected = {};
      this.selectMode = false;
    },
  },
});
