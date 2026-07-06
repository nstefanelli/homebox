import { beforeEach, describe, expect, test } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useLabelPrintQueue, type PrintQueueEntry } from "./labels";
import { useLabelSelection } from "./labelSelection";

const entry = (id: string): PrintQueueEntry => ({
  id,
  kind: "container",
  name: `Tote ${id}`,
  parentPath: "Garage",
  url: `https://hb.local/location/${id}`,
});

beforeEach(() => {
  setActivePinia(createPinia());
});

describe("useLabelPrintQueue", () => {
  test("set replaces, add appends and dedupes by id, clear empties", () => {
    const q = useLabelPrintQueue();
    q.set([entry("a"), entry("b")]);
    expect(q.entries).toHaveLength(2);

    q.add([entry("b"), entry("c")]);
    expect(q.entries.map(e => e.id)).toEqual(["a", "b", "c"]);

    q.clear();
    expect(q.entries).toHaveLength(0);
  });
});

describe("useLabelSelection", () => {
  test("toggle adds then removes; clear resets mode and selection", () => {
    const s = useLabelSelection();
    s.selectMode = true;

    s.toggle(entry("a"));
    s.toggle(entry("b"));
    expect(s.count).toBe(2);

    s.toggle(entry("a"));
    expect(s.count).toBe(1);
    expect(Object.keys(s.selected)).toEqual(["b"]);

    s.clear();
    expect(s.count).toBe(0);
    expect(s.selectMode).toBe(false);
  });
});
