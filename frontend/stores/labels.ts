import { defineStore } from "pinia";

export interface PrintQueueEntry {
  id: string;
  kind: "location" | "container" | "item";
  name: string;
  /** e.g. "Attic → Shelf B" — shown as the label's second line */
  parentPath?: string;
  /** formatted asset id (items only), e.g. "000-042" */
  assetId?: string;
  /** absolute deep-link URL encoded into the QR code */
  url: string;
}

export const useLabelPrintQueue = defineStore("label-print-queue", {
  state: () => ({
    entries: [] as PrintQueueEntry[],
  }),
  actions: {
    set(entries: PrintQueueEntry[]) {
      this.entries = [...entries];
    },
    add(entries: PrintQueueEntry[]) {
      const known = new Set(this.entries.map(e => e.id));
      this.entries.push(...entries.filter(e => !known.has(e.id)));
    },
    clear() {
      this.entries = [];
    },
  },
});
