<template>
  <div class="mt-4 flex flex-col gap-2">
    <div class="flex items-start gap-2">
      <div ref="inputWrap" class="grow">
        <Textarea
          v-model="input"
          :rows="inputRows"
          class="min-h-10"
          :placeholder="
            mode === 'items'
              ? $t('components.entity.quick_add.placeholder_items')
              : $t('components.entity.quick_add.placeholder_list')
          "
          :aria-label="$t('components.entity.quick_add.add')"
          @keydown="onKeydown"
          @paste="onPaste"
        />
      </div>
      <Button class="shrink-0" @click="submit">
        <MdiPlus />
        {{ $t("components.entity.quick_add.add") }}
      </Button>
      <ButtonGroup class="shrink-0">
        <Button
          :variant="mode === 'items' ? 'default' : 'outline'"
          :aria-pressed="mode === 'items'"
          data-pos="start"
          @click="mode = 'items'"
        >
          {{ $t("components.entity.quick_add.mode_items") }}
        </Button>
        <Button
          :variant="mode === 'list' ? 'default' : 'outline'"
          :aria-pressed="mode === 'list'"
          data-pos="end"
          @click="mode = 'list'"
        >
          {{ $t("components.entity.quick_add.mode_list") }}
        </Button>
      </ButtonGroup>
    </div>

    <div v-if="showProgress || chips.length > 0 || createdItems.length > 0" class="flex flex-wrap items-center gap-2">
      <span v-if="showProgress" class="text-sm text-muted-foreground">
        {{ $t("components.entity.quick_add.progress", { done: processedCount, total: totalCount }) }}
      </span>
      <Badge v-for="chip in chips" :key="chip.key" variant="secondary" class="gap-1 font-normal">
        <span>{{ chip.label }}</span>
        <button type="button" class="font-medium underline-offset-2 hover:underline" @click="undoChip(chip)">
          {{ $t("components.entity.quick_add.undo") }}
        </button>
      </Badge>
      <Button v-if="createdItems.length > 0" variant="outline" size="sm" class="ml-auto" @click="printLabels">
        <MdiPrinterOutline />
        {{ $t("components.entity.quick_add.print_labels", { count: createdItems.length }) }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import type { EntityOut } from "~~/lib/api/types/data-contracts";
  import { parseQuickAddLine, splitQuickAddLines } from "~~/lib/quick-add";
  import { itemQrPayload } from "~~/lib/labels/qr";
  import { useLabelPrintQueue } from "~~/stores/labels";
  import { useEntityTypeStore } from "~~/stores/entityTypes";
  import MdiPlus from "~icons/mdi/plus";
  import MdiPrinterOutline from "~icons/mdi/printer-outline";
  import { Button, ButtonGroup } from "~/components/ui/button";
  import { Badge } from "~/components/ui/badge";
  import { Textarea } from "~/components/ui/textarea";

  type QuickAddMode = "items" | "list";

  type QuickAddChip =
    | { key: number; label: string; kind: "item"; entity: EntityOut }
    | { key: number; label: string; kind: "line"; line: string };

  const props = defineProps<{
    /** the location/container the row adds into (the page's loaded entity) */
    entity: EntityOut;
  }>();

  const emit = defineEmits<{
    /** fired after a run creates items (and after an item undo) so the page refetches its items list */
    (e: "refresh"): void;
    /** fired on every optimistic/reverted contents change so the page's Contents card stays in sync */
    (e: "update:contents", contents: string): void;
  }>();

  const { t } = useI18n();
  const api = useUserApi();
  const entityTypeStore = useEntityTypeStore();
  const printQueue = useLabelPrintQueue();

  onMounted(() => {
    void entityTypeStore.ensureFetched().catch(() => {
      // Creation surfaces the missing-type toast if the fetch never lands.
    });
  });

  const input = ref("");
  // Default is fixed to items each mount by design -- no persistence.
  const mode = ref<QuickAddMode>("items");
  const inputWrap = ref<HTMLDivElement | null>(null);

  // Multi-line only when failed lines were restored -- grow to show them.
  const inputRows = computed(() => Math.min(6, input.value.split("\n").length));

  // ---------------------------------------------------------------------------
  // Sequential queue. Every entry (typed Enter, Add click, multi-line paste)
  // lands here with the mode captured at enqueue time; a single in-flight
  // processQueue() drains it one create/update at a time, so an Enter during a
  // paste run queues behind it instead of interleaving (orderly asset IDs).
  // ---------------------------------------------------------------------------
  const queue: { text: string; mode: QuickAddMode }[] = [];
  const running = ref(false);
  const processedCount = ref(0);
  const totalCount = ref(0);
  const failedLines = ref<string[]>([]);

  const showProgress = computed(() => running.value && totalCount.value > 1);

  // Session state: items created via quick add (drives the Print labels
  // counter) and the undoable chips (created items + appended list lines).
  const createdItems = ref<EntityOut[]>([]);
  const chips = ref<QuickAddChip[]>([]);
  let chipKey = 0;

  // Working copy of the entity's contents manifest for List mode
  // (read-modify-write, last-write-wins -- single-user app by design).
  const contents = ref(props.entity.contents ?? "");
  watch(
    () => props.entity.contents,
    value => {
      contents.value = value ?? "";
    }
  );

  function focusInput() {
    inputWrap.value?.querySelector("textarea")?.focus();
  }

  function onKeydown(e: KeyboardEvent) {
    // Enter submits (keyboard-first loop); Shift+Enter falls through to insert
    // a newline for hand-built multi-line entries.
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  }

  function onPaste(e: ClipboardEvent) {
    const text = e.clipboardData?.getData("text") ?? "";
    if (!text.includes("\n")) {
      return; // single-line paste behaves like typing
    }
    e.preventDefault();
    enqueue(splitQuickAddLines(text));
  }

  function submit() {
    const lines = splitQuickAddLines(input.value);
    if (lines.length === 0) {
      return; // whitespace-only no-ops
    }
    input.value = "";
    focusInput();
    enqueue(lines);
  }

  function enqueue(lines: string[]) {
    if (lines.length === 0) return;
    for (const text of lines) {
      queue.push({ text, mode: mode.value });
    }
    totalCount.value += lines.length;
    if (!running.value) {
      void processQueue();
    }
  }

  // Warn once per run, not once per line -- a 15-line paste into a collection
  // with no item type would otherwise stack 15 identical toasts.
  let warnedNoItemType = false;

  async function processQueue() {
    running.value = true;
    warnedNoItemType = false;
    let createdAny = false;
    try {
      while (queue.length > 0) {
        const entry = queue.shift()!;
        const ok = entry.mode === "items" ? await createItem(entry.text) : await appendLine(entry.text);
        if (ok && entry.mode === "items") {
          createdAny = true;
        } else if (!ok) {
          failedLines.value.push(entry.text);
        }
        processedCount.value++;
      }
    } finally {
      running.value = false;
      processedCount.value = 0;
      totalCount.value = 0;

      // Per-line failure handling: only the failed lines come back into the
      // input (ahead of anything typed mid-run), so typed content is never lost.
      if (failedLines.value.length > 0) {
        const pending = input.value.trim();
        input.value = failedLines.value.join("\n") + (pending ? "\n" + pending : "");
        toast.error(t("components.entity.quick_add.toast.lines_failed", { count: failedLines.value.length }));
        failedLines.value = [];
      }

      if (createdAny) {
        emit("refresh");
      }
    }
  }

  // ---------------------------------------------------------------------------
  // Items mode: one POST /v1/entities per line, same defaults as the create
  // modal's plain-item path -- default Item entity type, parent = this
  // location, quantity from the parsed prefix.
  // ---------------------------------------------------------------------------
  async function createItem(line: string): Promise<boolean> {
    const itemType = entityTypeStore.allTypes.find(type => !type.isLocation);
    if (!itemType?.id) {
      if (!warnedNoItemType) {
        warnedNoItemType = true;
        toast.error(t("components.entity.quick_add.toast.no_item_type"));
      }
      return false;
    }

    const { name, quantity } = parseQuickAddLine(line);

    try {
      const { data, error } = await api.items.create({
        name,
        quantity,
        description: "",
        manufacturer: "",
        modelNumber: "",
        icon: "",
        tagIds: [],
        parentId: props.entity.id,
        entityTypeId: itemType.id,
      });
      if (error || !data) {
        return false;
      }
      createdItems.value.push(data);
      chips.value.push({
        key: chipKey++,
        kind: "item",
        entity: data,
        label: quantity > 1 ? `${name} ×${quantity}` : name,
      });
      return true;
    } catch {
      return false;
    }
  }

  // ---------------------------------------------------------------------------
  // List mode: append one literal line (no quantity parsing) to the entity's
  // contents manifest via update. Optimistic: the local copy (and the page's
  // Contents card, via the emit) updates immediately and reverts on failure.
  // ---------------------------------------------------------------------------
  async function appendLine(line: string): Promise<boolean> {
    const previous = contents.value;
    const next = previous ? `${previous}\n${line}` : line;

    contents.value = next;
    emit("update:contents", next);

    if (!(await saveContents(next))) {
      contents.value = previous;
      emit("update:contents", previous);
      return false;
    }

    chips.value.push({ key: chipKey++, kind: "line", line, label: line });
    return true;
  }

  async function saveContents(next: string): Promise<boolean> {
    // Mirrors the full-entity update pattern used elsewhere (e.g. the create
    // modal's renameToAssetId): spread the loaded entity, override the edges
    // the PUT body needs in id/flat form.
    const entity = props.entity;
    try {
      const { error } = await api.items.update(entity.id, {
        ...entity,
        contents: next,
        parentId: entity.parent?.id ?? null,
        entityTypeId: entity.entityType?.id,
        tagIds: entity.tags.map(tag => tag.id),
      });
      return !error;
    } catch {
      return false;
    }
  }

  // ---------------------------------------------------------------------------
  // Undo. Items: real DELETE, then drop from the session set + refresh the
  // list. Lines: remove the last occurrence from the manifest via update.
  // ---------------------------------------------------------------------------
  async function undoChip(chip: QuickAddChip) {
    if (chip.kind === "item") {
      try {
        const { error } = await api.items.delete(chip.entity.id);
        if (error) throw error;
      } catch {
        toast.error(t("components.entity.quick_add.toast.undo_failed"));
        return;
      }
      createdItems.value = createdItems.value.filter(item => item.id !== chip.entity.id);
      chips.value = chips.value.filter(c => c.key !== chip.key);
      emit("refresh");
      return;
    }

    const previous = contents.value;
    const lines = previous.split("\n");
    const at = lines.lastIndexOf(chip.line);
    if (at !== -1) {
      lines.splice(at, 1);
    }
    const next = lines.join("\n");

    contents.value = next;
    emit("update:contents", next);

    if (!(await saveContents(next))) {
      contents.value = previous;
      emit("update:contents", previous);
      toast.error(t("components.entity.quick_add.toast.undo_failed"));
      return;
    }

    chips.value = chips.value.filter(c => c.key !== chip.key);
  }

  // Label wrap-up: enqueue exactly this session's quick-added items (assetId
  // threaded from the create responses) -- same queue + generator flow as the
  // create modal's batch toast.
  function printLabels() {
    printQueue.set(
      createdItems.value.map(item => ({
        id: item.id,
        kind: "item" as const,
        name: item.name,
        parentPath: props.entity.name ?? "",
        assetId: item.assetId,
        // itemQrPayload deep-links /item/{id} when the item has no asset id --
        // `/a/` with an empty id is a dead link.
        url: itemQrPayload(window.location.origin, item),
      }))
    );
    navigateTo("/reports/label-generator");
  }
</script>
