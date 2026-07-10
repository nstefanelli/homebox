<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import DOMPurify from "dompurify";
  import { route } from "../../lib/api/base";
  import { calculateGrid } from "../../lib/labels/grid";
  import type { AcceptableValue } from "reka-ui";
  import { LABEL_PRESETS, CUSTOM_PRESET_ID } from "~~/lib/labels/presets";
  import { useLabelPrintQueue } from "~~/stores/labels";
  import { Toaster, toast } from "@/components/ui/sonner";
  import { Separator } from "@/components/ui/separator";
  import { Button } from "@/components/ui/button";
  import { Label } from "@/components/ui/label";
  import { Input } from "@/components/ui/input";
  import { Checkbox } from "@/components/ui/checkbox";
  import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

  const { t } = useI18n();

  definePageMeta({
    middleware: ["auth"],
    layout: false,
  });
  useHead({
    title: "HomeBox | " + t("reports.label_generator.title"),
  });

  const api = useUserApi();

  const bordered = ref(false);
  const printLocationRow = ref(true);
  const labelBlankLine = "_______________";

  // Print-safe inset inside each label cell. The sheet prints full-bleed
  // (@page margin:0), so the outer columns' die-cut edges sit ~0.19in from the
  // paper edge — inside many printers' unprintable zone. Content flush to the
  // label edge gets shaved there (and is vulnerable to die-cut registration
  // drift anyway). Absolute CSS unit on purpose: applies regardless of the
  // sheet's measure setting. Bordered mode still draws at the true label edge
  // (it marks the die-cut for alignment tests).
  const LABEL_SAFE_INSET = "0.1in";

  // Behavior constants for HomeBox text replacement
  const BEHAVIOR_SHOW = "show";
  const BEHAVIOR_ALWAYS_REPLACE = "always_replace";
  const BEHAVIOR_ITEM_NO_NAME_NO_LOCATION = "item_no_name_no_location";
  const BEHAVIOR_ITEM_NO_NAME = "item_no_name";
  const BEHAVIOR_ITEM_NO_LOCATION = "item_no_location";

  const replaceHomeboxBehavior = ref(BEHAVIOR_SHOW);
  const replaceHomeboxText = ref(labelBlankLine);

  const displayProperties = reactive({
    baseURL: window.location.origin,
    assetRange: 1,
    assetRangeMax: 91,
    skipLabels: 0,
    measure: "in",
    gapY: 0.25,
    columns: 3,
    cardHeight: 1,
    cardWidth: 2.63,
    pageWidth: 8.5,
    pageHeight: 11,
    pageTopPadding: 0.52,
    pageBottomPadding: 0.42,
    pageLeftPadding: 0.25,
    pageRightPadding: 0.1,
  });

  // Print queue (Task 9 stores): when non-empty, the page renders the queued
  // entries (containers/locations/items picked elsewhere) instead of the
  // legacy asset-range labels.
  const printQueue = useLabelPrintQueue();
  const queueMode = computed(() => printQueue.entries.length > 0);

  // Sheet presets (Task 8 module): last choice persisted to localStorage.
  // "measure" is bundled with the other dimension fields since presets fix it too.
  const PRESET_STORAGE_KEY = "homebox:labelPreset";
  const selectedPresetId = ref<string>(localStorage.getItem(PRESET_STORAGE_KEY) ?? "avery-5160");
  const DIMENSION_REFS = new Set<keyof typeof displayProperties>([
    "measure",
    "cardHeight",
    "cardWidth",
    "pageWidth",
    "pageHeight",
    "pageTopPadding",
    "pageBottomPadding",
    "pageLeftPadding",
    "pageRightPadding",
  ]);
  const currentPreset = computed(() =>
    selectedPresetId.value === CUSTOM_PRESET_ID ? undefined : LABEL_PRESETS.find(p => p.id === selectedPresetId.value)
  );

  function applyPreset(id: string) {
    selectedPresetId.value = id;
    localStorage.setItem(PRESET_STORAGE_KEY, id);
    const p = currentPreset.value;
    if (p) {
      displayProperties.measure = p.measure;
      displayProperties.cardWidth = p.labelWidth;
      displayProperties.cardHeight = p.labelHeight;
      displayProperties.pageWidth = p.pageWidth;
      displayProperties.pageHeight = p.pageHeight;
      displayProperties.pageTopPadding = p.pagePaddingTop;
      displayProperties.pageBottomPadding = p.pagePaddingBottom;
      displayProperties.pageLeftPadding = p.pagePaddingLeft;
      displayProperties.pageRightPadding = p.pagePaddingRight;
    }
    // Custom keeps whatever dims are currently set (just unlocks the inputs),
    // but pages are always recalculated on a preset switch.
    calcPages();
  }

  // Select's update:model-value emits the broader AcceptableValue type; narrow
  // to string (preset/custom ids are always strings) before calling applyPreset.
  function onPresetSelect(value: AcceptableValue) {
    if (typeof value === "string") {
      applyPreset(value);
    }
  }

  interface InputDef {
    label: string;
    ref: keyof typeof displayProperties;
    type?: "number" | "text";
    min?: number;
    step?: number;
  }

  const propertyInputs = computed<InputDef[]>(() => {
    const defs: InputDef[] = [
      {
        label: t("reports.label_generator.asset_start"),
        ref: "assetRange",
      },
      {
        label: t("reports.label_generator.asset_end"),
        ref: "assetRangeMax",
      },
      {
        label: t("reports.label_generator.skip_first_labels"),
        ref: "skipLabels",
        min: 0,
        step: 1,
      },
      {
        label: t("reports.label_generator.measure_type"),
        ref: "measure",
        type: "text",
      },
      {
        label: t("reports.label_generator.label_height"),
        ref: "cardHeight",
      },
      {
        label: t("reports.label_generator.label_width"),
        ref: "cardWidth",
      },
      {
        label: t("reports.label_generator.page_width"),
        ref: "pageWidth",
      },
      {
        label: t("reports.label_generator.page_height"),
        ref: "pageHeight",
      },
      {
        label: t("reports.label_generator.page_top_padding"),
        ref: "pageTopPadding",
      },
      {
        label: t("reports.label_generator.page_bottom_padding"),
        ref: "pageBottomPadding",
      },
      {
        label: t("reports.label_generator.page_left_padding"),
        ref: "pageLeftPadding",
      },
      {
        label: t("reports.label_generator.page_right_padding"),
        ref: "pageRightPadding",
      },
      {
        label: t("reports.label_generator.base_url"),
        ref: "baseURL",
        type: "text",
      },
    ];
    // In queue mode the labels come from the print queue, not the asset range.
    return queueMode.value ? defs.filter(d => d.ref !== "assetRange" && d.ref !== "assetRangeMax") : defs;
  });

  type LabelData = {
    url: string;
    /** asset id (items) or name (locations/containers) */
    topLine: string;
    /** item name; empty for locations/containers */
    nameLine: string;
    /** existing location row / parentPath */
    locationLine: string;
  };

  function fmtAssetID(aid: number | string) {
    aid = aid.toString();

    let aidStr = aid.toString().padStart(6, "0");
    aidStr = aidStr.slice(0, 3) + "-" + aidStr.slice(3);
    return aidStr;
  }

  function getQRCodeUrl(assetID: string): string {
    let origin = displayProperties.baseURL.trim();

    // remove trailing slash
    if (origin.endsWith("/")) {
      origin = origin.slice(0, -1);
    }

    const data = `${origin}/a/${assetID}`;

    return route(`/qrcode`, { data: encodeURIComponent(data) });
  }

  // Generalized QR helper for the print queue: entries already carry an
  // absolute deep-link URL, so no origin/`/a/` composition is needed.
  function getQRCodeUrlFor(payload: string): string {
    return route("/qrcode", { data: encodeURIComponent(payload) });
  }

  function getItem(n: number, item: { assetId: string; name: string; location: { name: string } } | null): LabelData {
    // format n into - seperated string with leading zeros
    const assetID = fmtAssetID(item?.assetId ?? n + 1);

    return {
      url: getQRCodeUrl(assetID),
      topLine: item?.assetId ?? assetID,
      nameLine: item?.name ?? labelBlankLine,
      locationLine: item?.location?.name ?? labelBlankLine,
    };
  }

  const { data: allFields } = await useAsyncData(async () => {
    const { data, error } = await api.items.getAll({ orderBy: "assetId" });

    if (error) {
      return {
        items: [],
      };
    }

    return data;
  });

  const items = computed(() => {
    if (displayProperties.assetRange > displayProperties.assetRangeMax) {
      return [];
    }

    const diff = displayProperties.assetRangeMax - displayProperties.assetRange;

    if (diff > 999) {
      return [];
    }

    const items: LabelData[] = [];
    for (let i = displayProperties.assetRange - 1; i < displayProperties.assetRangeMax - 1; i++) {
      const item = allFields?.value?.items?.[i];
      if (item?.location) {
        items.push(getItem(i, item as { assetId: string; location: { name: string }; name: string }));
      } else {
        items.push(getItem(i, null));
      }
    }
    return items;
  });

  // Print-queue labels, fed through the SAME calcPages pagination path as the
  // asset-range items below (see calcPages' sourceItems selection).
  const queueLabels = computed<LabelData[]>(() =>
    printQueue.entries.map(e => ({
      url: getQRCodeUrlFor(e.url),
      topLine: e.kind === "item" ? (e.assetId ?? e.name) : e.name,
      nameLine: e.kind === "item" ? e.name : "",
      locationLine: e.parentPath ?? "",
    }))
  );

  const getHomeBoxLineText = computed(() => {
    return (item: LabelData): string | null => {
      if (replaceHomeboxBehavior.value === BEHAVIOR_SHOW) {
        return "HomeBox";
      }
      if (replaceHomeboxBehavior.value === BEHAVIOR_ALWAYS_REPLACE) {
        return replaceHomeboxText.value;
      }
      if (
        replaceHomeboxBehavior.value === BEHAVIOR_ITEM_NO_NAME_NO_LOCATION &&
        item.nameLine === labelBlankLine &&
        item.locationLine === labelBlankLine
      ) {
        return replaceHomeboxText.value;
      }
      if (replaceHomeboxBehavior.value === BEHAVIOR_ITEM_NO_NAME && item.nameLine === labelBlankLine) {
        return replaceHomeboxText.value;
      }
      if (replaceHomeboxBehavior.value === BEHAVIOR_ITEM_NO_LOCATION && item.locationLine === labelBlankLine) {
        return replaceHomeboxText.value;
      }
      return null;
    };
  });

  // Flat per-page label array (rendered via CSS grid — see template), sized
  // implicitly to `perPage` items with the final page possibly shorter.
  type Page = {
    items: Array<LabelData | null>;
  };

  const pages = ref<Page[]>([]);

  const out = ref({
    measure: "in",
    cols: 0,
    rows: 0,
    gapY: 0,
    gapX: 0,
    card: {
      width: 0,
      height: 0,
    },
    page: {
      width: 0,
      height: 0,
      pt: 0,
      pb: 0,
      pl: 0,
      pr: 0,
    },
  });

  // The sheet renders at the full physical page width (e.g. 8.5in) with the
  // label margins baked into its own padding. Without an explicit `@page` rule
  // the browser reserves its own default print margins, so the full-width sheet
  // no longer fits the printable area — Safari clips the outer label columns
  // left and right, Chrome shrinks-to-fit. Emitting `@page { size; margin: 0 }`
  // sized to the current sheet makes it print 1:1, edge to edge. Empty until a
  // sheet is generated (size 0 would be an invalid rule).
  const printPageRule = computed(() => {
    const p = out.value.page;
    if (!p.width || !p.height) return "";
    const m = out.value.measure;
    return `@page { size: ${p.width}${m} ${p.height}${m}; margin: 0; }`;
  });

  // Function form so unhead re-evaluates when the sheet is (re)generated —
  // passing the ComputedRef as a nested value does not get unwrapped.
  useHead(() => ({
    style: printPageRule.value ? [{ id: "label-page-rule", innerHTML: printPageRule.value }] : [],
  }));

  function calcPages() {
    // Set Out Dimensions
    const measureRegex = /in|cm|mm/;
    const measure = measureRegex.test(displayProperties.measure) ? displayProperties.measure : "in";

    const availablePageWidth =
      displayProperties.pageWidth - displayProperties.pageLeftPadding - displayProperties.pageRightPadding;
    const availablePageHeight =
      displayProperties.pageHeight - displayProperties.pageTopPadding - displayProperties.pageBottomPadding;

    if (availablePageWidth < displayProperties.cardWidth || availablePageHeight < displayProperties.cardHeight) {
      toast.error(t("reports.label_generator.toast.page_too_small_card"));
      // Keep the previous out.value (matches prior behavior of calculateGridData).
    } else {
      const preset = currentPreset.value;
      const grid = calculateGrid({
        pageWidth: displayProperties.pageWidth,
        pageHeight: displayProperties.pageHeight,
        cardWidth: displayProperties.cardWidth,
        cardHeight: displayProperties.cardHeight,
        pagePaddingTop: displayProperties.pageTopPadding,
        pagePaddingBottom: displayProperties.pageBottomPadding,
        pagePaddingLeft: displayProperties.pageLeftPadding,
        pagePaddingRight: displayProperties.pageRightPadding,
        // Preset selected => explicit-gutter mode (fixed physical gaps).
        // Custom => no gutterX/gutterY passed, preserves derived-gap behavior.
        ...(preset ? { gutterX: preset.gutterX, gutterY: preset.gutterY } : {}),
      });

      out.value = {
        measure,
        cols: grid.cols,
        rows: grid.rows,
        gapX: grid.gapX,
        gapY: grid.gapY,
        card: {
          width: displayProperties.cardWidth,
          height: displayProperties.cardHeight,
        },
        page: {
          width: displayProperties.pageWidth,
          height: displayProperties.pageHeight,
          pt: displayProperties.pageTopPadding,
          pb: displayProperties.pageBottomPadding,
          pl: displayProperties.pageLeftPadding,
          pr: displayProperties.pageRightPadding,
        },
      };
    }

    const calc: Page[] = [];

    const perPage = out.value.rows * out.value.cols;
    const maxSkipLabels = Math.max(0, perPage - 1);

    const skipLabelsRaw = Number(displayProperties.skipLabels);
    const skipLabels = Number.isFinite(skipLabelsRaw)
      ? Math.min(maxSkipLabels, Math.max(0, Math.floor(skipLabelsRaw)))
      : 0;
    if (Number(displayProperties.skipLabels) !== skipLabels) {
      displayProperties.skipLabels = skipLabels;
    }

    // Queue mode feeds queuedLabels through this exact same pagination path
    // (skip-padding then chunk into pages) — not a separate code path — so
    // skipLabels behaves identically in both modes.
    const sourceItems = queueMode.value ? queueLabels.value : items.value;
    if (sourceItems.length === 0) {
      pages.value = [];
      return;
    }

    const itemsCopy: Array<LabelData | null> = [...sourceItems];
    if (skipLabels > 0) {
      itemsCopy.unshift(...Array.from({ length: skipLabels }, () => null));
    }

    while (itemsCopy.length > 0) {
      const page: Page = {
        items: [],
      };

      for (let i = 0; i < perPage; i++) {
        const item = itemsCopy.shift();
        if (typeof item === "undefined") {
          break;
        }

        page.items.push(item ?? null);
      }

      calc.push(page);
    }

    pages.value = calc;
  }

  onMounted(() => applyPreset(selectedPresetId.value));
</script>

<template>
  <div class="print:hidden">
    <Toaster />
    <div class="container prose mx-auto max-w-4xl p-4 pt-6">
      <h1>HomeBox {{ $t("reports.label_generator.title") }}</h1>
      <p>
        {{ $t("reports.label_generator.instruction_1") }}
      </p>
      <p>
        {{ $t("reports.label_generator.instruction_2") }}
      </p>
      <p v-html="DOMPurify.sanitize($t('reports.label_generator.instruction_3'))" />
      <h2>{{ $t("reports.label_generator.tips") }}</h2>
      <ul>
        <li v-html="DOMPurify.sanitize($t('reports.label_generator.tip_1'))" />
        <li v-html="DOMPurify.sanitize($t('reports.label_generator.tip_2'))" />
        <li v-html="DOMPurify.sanitize($t('reports.label_generator.tip_3'))" />
      </ul>
      <div class="flex flex-wrap gap-2">
        <NuxtLink href="/collection/tools">{{ $t("collection.tabs.tools") }}</NuxtLink>
        <NuxtLink href="/home">{{ $t("menu.home") }}</NuxtLink>
      </div>
    </div>
    <Separator class="mx-auto max-w-4xl" />
    <div class="container mx-auto max-w-4xl p-4">
      <div v-if="queueMode" class="mb-4 flex w-full max-w-xs items-center gap-4">
        <p>{{ $t("reports.label_generator.queue_count", { count: printQueue.entries.length }) }}</p>
        <Button
          variant="outline"
          @click="
            printQueue.clear();
            calcPages();
          "
        >
          {{ $t("reports.label_generator.clear_queue") }}
        </Button>
      </div>
      <div class="mb-4 flex w-full max-w-xs flex-col">
        <Label for="select-labelPreset">
          {{ $t("reports.label_generator.presets.title") }}
        </Label>
        <Select
          id="select-labelPreset"
          :model-value="selectedPresetId"
          class="w-full max-w-xs"
          @update:model-value="onPresetSelect"
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="preset in LABEL_PRESETS" :key="preset.id" :value="preset.id">
              {{ $t(`reports.label_generator.presets.${preset.nameKey}`) }}
            </SelectItem>
            <SelectItem :value="CUSTOM_PRESET_ID">
              {{ $t("reports.label_generator.presets.custom") }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div class="mx-auto grid grid-cols-2 gap-3">
        <div v-for="(prop, i) in propertyInputs" :key="i" class="flex w-full max-w-xs flex-col">
          <Label :for="`input-${prop.ref}`">
            {{ prop.label }}
          </Label>
          <Input
            :id="`input-${prop.ref}`"
            v-model="displayProperties[prop.ref]"
            :type="prop.type ? prop.type : 'number'"
            :min="prop.min"
            :max="prop.ref === 'skipLabels' ? Math.max(0, out.rows * out.cols - 1) : undefined"
            :step="prop.type === 'text' ? undefined : (prop.step ?? 0.01)"
            :disabled="DIMENSION_REFS.has(prop.ref) && selectedPresetId !== CUSTOM_PRESET_ID"
            :placeholder="$t('reports.label_generator.input_placeholder')"
            class="w-full max-w-xs"
          />
        </div>
        <div class="flex w-full max-w-xs flex-col">
          <Label for="select-replaceHomeboxBehavior">
            {{ $t("reports.label_generator.replace_homebox_behavior") }}
          </Label>
          <Select id="select-replaceHomeboxBehavior" v-model="replaceHomeboxBehavior" class="w-full max-w-xs">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem :value="BEHAVIOR_SHOW">
                {{ $t("reports.label_generator.replace_homebox_behavior_show_homebox") }}
              </SelectItem>
              <SelectItem :value="BEHAVIOR_ITEM_NO_NAME_NO_LOCATION">
                {{ $t("reports.label_generator.replace_homebox_behavior_item_no_name_no_location") }}
              </SelectItem>
              <SelectItem :value="BEHAVIOR_ITEM_NO_NAME">
                {{ $t("reports.label_generator.replace_homebox_behavior_item_no_name") }}
              </SelectItem>
              <SelectItem :value="BEHAVIOR_ITEM_NO_LOCATION">
                {{ $t("reports.label_generator.replace_homebox_behavior_item_no_location") }}
              </SelectItem>
              <SelectItem :value="BEHAVIOR_ALWAYS_REPLACE">
                {{ $t("reports.label_generator.replace_homebox_behavior_always_replace") }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div v-if="replaceHomeboxBehavior !== BEHAVIOR_SHOW" class="flex w-full max-w-xs flex-col">
          <Label for="input-replaceHomeboxText">
            {{ $t("reports.label_generator.replace_homebox_text") }}
          </Label>
          <Input
            id="input-replaceHomeboxText"
            v-model="replaceHomeboxText"
            type="text"
            :placeholder="$t('reports.label_generator.input_placeholder')"
            class="w-full max-w-xs"
          />
        </div>
      </div>
      <div class="max-w-xs">
        <div class="flex items-center gap-2 py-4">
          <Checkbox id="borderedLabels" v-model="bordered" />
          <Label class="cursor-pointer" for="borderedLabels">
            {{ $t("reports.label_generator.bordered_labels") }}
          </Label>
        </div>
        <div class="flex items-center gap-2 py-4">
          <Checkbox id="printLocationRow" v-model="printLocationRow" />
          <Label class="cursor-pointer" for="printLocationRow">
            {{ $t("reports.label_generator.print_location_row") }}
          </Label>
        </div>
      </div>

      <div>
        <p>{{ $t("reports.label_generator.qr_code_example") }} {{ displayProperties.baseURL }}/a/{asset_id}</p>
        <Button size="lg" class="my-4 w-full" @click="calcPages">
          {{ $t("reports.label_generator.generate_page") }}
        </Button>
      </div>
    </div>
  </div>
  <div class="flex flex-col items-center">
    <section
      v-for="(page, pi) in pages"
      :key="pi"
      class="border-2 print:border-none"
      :style="{
        paddingTop: `${out.page.pt}${out.measure}`,
        paddingBottom: `${out.page.pb}${out.measure}`,
        paddingLeft: `${out.page.pl}${out.measure}`,
        paddingRight: `${out.page.pr}${out.measure}`,
        width: `${out.page.width}${out.measure}`,
        display: 'grid',
        gridTemplateColumns: `repeat(${out.cols}, ${out.card.width}${out.measure})`,
        gridAutoRows: `${out.card.height}${out.measure}`,
        columnGap: `${out.gapX}${out.measure}`,
        // Derived gapY was never rendered pre-preset-feature (old flex rows made rowGap
        // visually inert); rendering it in Custom mode overflows the page since the
        // formula spans the full unpadded pageHeight. Only presets carry a real gutter.
        rowGap: `${currentPreset ? out.gapY : 0}${out.measure}`,
        background: `white`,
        color: `black`,
        // Force every sheet after the first onto a fresh physical page. Without
        // this the sections flow continuously: a full 11in section plus the next
        // one's top padding straddle the page boundary, so page 2+ prints its
        // labels shifted ~0.25in up and they no longer register on the Avery
        // stock. break-before (not break-after) avoids a trailing blank page.
        breakBefore: pi > 0 ? 'page' : 'auto',
      }"
    >
      <div
        v-for="(item, idx) in page.items"
        :key="idx"
        class="flex break-inside-avoid border-2"
        :class="{
          'border-black': bordered && !!item,
          'border-transparent': !bordered || !item,
        }"
        :style="{
          height: `${out.card.height}${out.measure}`,
          width: `${out.card.width}${out.measure}`,
          paddingLeft: LABEL_SAFE_INSET,
          paddingRight: LABEL_SAFE_INSET,
        }"
      >
        <template v-if="item">
          <div class="flex items-center">
            <img
              :src="item.url"
              :style="{
                minWidth: `${out.card.height * 0.9}${out.measure}`,
                width: `${out.card.height * 0.9}${out.measure}`,
                height: `${out.card.height * 0.9}${out.measure}`,
              }"
            />
          </div>
          <div class="ml-2 flex min-w-0 flex-col justify-center overflow-hidden">
            <div class="font-bold">{{ item.topLine }}</div>
            <div
              v-if="getHomeBoxLineText(item)"
              class="text-xs"
              :class="{ 'font-light italic': getHomeBoxLineText(item) !== labelBlankLine }"
            >
              {{ getHomeBoxLineText(item) }}
            </div>
            <div class="overflow-hidden text-wrap text-xs">{{ item.nameLine }}</div>
            <div v-if="printLocationRow" class="text-xs">{{ item.locationLine }}</div>
          </div>
        </template>
      </div>
    </section>
  </div>
</template>
