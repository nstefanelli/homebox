import { defineStore } from "pinia";

export interface PrintQueueEntry {
  id: string;
  kind: "location" | "container" | "item";
  name: string;
  /** e.g. "Attic → Shelf B" — shown as the label's second line */
  parentPath?: string;
  /**
   * formatted asset id, e.g. "000-042" — any kind, not just items; the label
   * prints it as the bold top line and omits the row when empty/absent
   */
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
      for (const entry of entries) {
        if (known.has(entry.id)) continue;
        known.add(entry.id);
        this.entries.push(entry);
      }
    },
    clear() {
      this.entries = [];
    },
  },
});
