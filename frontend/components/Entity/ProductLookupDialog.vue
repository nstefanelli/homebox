<template>
  <DialogRoot :open="open" @update:open="onOpenChange">
    <DialogContent class="w-full md:max-w-xl lg:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{{ $t("components.entity.product_lookup.title") }}</DialogTitle>
        <DialogDescription class="sr-only">
          {{ $t("components.entity.product_lookup.title") }}
        </DialogDescription>
      </DialogHeader>

      <form class="flex items-end gap-3" @submit.prevent="search()">
        <FormTextField
          v-model="keyword"
          class="grow"
          :label="$t('components.entity.product_lookup.keyword_label')"
          :max-length="200"
          :disabled="searching || aiLoading"
        />
        <Button type="submit" class="h-10" :disabled="searching || aiLoading || !keyword.trim()">
          <MdiLoading v-if="searching" class="animate-spin" />
          <MdiMagnify v-else class="size-5" />
          {{ $t("components.entity.product_lookup.search") }}
        </Button>
      </form>

      <div
        v-if="errorKind"
        class="flex items-center gap-2 rounded-md border border-destructive bg-destructive/10 p-4 text-destructive"
        role="alert"
      >
        <MdiAlertCircleOutline class="shrink-0" />
        <span class="text-sm font-medium">{{ errorMessage }}</span>
      </div>

      <div v-if="aiLoading" class="flex items-center gap-2 text-sm text-muted-foreground">
        <MdiLoading class="size-4 animate-spin" />
        <span>{{ $t("components.entity.product_lookup.ai_loading") }}</span>
      </div>

      <template v-if="candidates.length > 0">
        <div class="flex max-h-[50vh] flex-col gap-2 overflow-y-auto">
          <button
            v-for="(candidate, index) in candidates"
            :key="index"
            type="button"
            class="flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors hover:border-primary hover:bg-accent"
            @click="pick(candidate)"
          >
            <img
              v-if="candidate.product.imageBase64"
              :src="candidate.product.imageBase64"
              class="size-16 shrink-0 rounded object-contain shadow-sm"
              :alt="candidate.product.item.name"
            />
            <div v-else class="flex size-16 shrink-0 items-center justify-center rounded bg-secondary">
              <MdiImageOutline class="size-6 text-muted-foreground" />
            </div>
            <div class="min-w-0 grow">
              <p class="truncate text-sm font-medium">{{ candidate.product.item.name }}</p>
              <p v-if="brandOf(candidate.product)" class="truncate text-sm text-muted-foreground">
                {{ brandOf(candidate.product) }}
              </p>
            </div>
            <Badge variant="secondary" class="shrink-0">
              {{
                candidate.aiGuess ? $t("components.entity.create_modal.ai_badge") : candidate.product.search_engine_name
              }}
            </Badge>
          </button>
        </div>
      </template>

      <p v-else-if="noResults" class="text-sm text-muted-foreground">
        {{ $t("components.entity.product_lookup.no_results") }}
      </p>

      <DialogFooter v-if="showAskAi">
        <Button type="button" variant="outline" :disabled="searching || aiLoading" @click="askAi()">
          <MdiCreation class="size-4" />
          {{ $t("components.entity.product_lookup.ask_ai") }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </DialogRoot>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { DialogRoot } from "reka-ui";
  import { Button } from "~/components/ui/button";
  import { Badge } from "~/components/ui/badge";
  import { DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
  import FormTextField from "@/components/Form/TextField.vue";
  import MdiAlertCircleOutline from "~icons/mdi/alert-circle-outline";
  import MdiCreation from "~icons/mdi/creation";
  import MdiImageOutline from "~icons/mdi/image-outline";
  import MdiLoading from "~icons/mdi/loading";
  import MdiMagnify from "~icons/mdi/magnify";
  import type { BarcodeProduct } from "~~/lib/api/types/data-contracts";
  import type { ProductLookupPick } from "~~/lib/enrich";
  import { useIntegrationsStore } from "~~/stores/integrations";

  /**
   * Keyword product lookup dialog: provider search first, one-click AI
   * fallback ("Ask AI instead") when AI is configured. Unlike the barcode
   * import dialog this is NOT a dialog-provider singleton — the provider
   * supports a single active dialog, and this one must stack on top of the
   * still-open create modal (and be instantiable per page for enrichment) —
   * so it is a plain v-model:open reka dialog that emits the chosen
   * candidate to whoever mounted it.
   */

  const props = defineProps<{
    open: boolean;
    /** Initial keyword each time the dialog opens (e.g. the typed item name). */
    seed?: string;
  }>();

  const emit = defineEmits<{
    "update:open": [value: boolean];
    pick: [candidate: ProductLookupPick];
  }>();

  const { t } = useI18n();
  const api = useUserApi();

  const integrationsStore = useIntegrationsStore();
  void integrationsStore.ensureFetched().catch(() => {
    // "Ask AI instead" stays hidden when the capability check fails.
  });
  const aiConfigured = computed(() => integrationsStore.aiConfigured);

  const keyword = ref("");
  const searching = ref(false);
  const aiLoading = ref(false);
  const results = ref<BarcodeProduct[] | null>(null);
  const aiCandidate = ref<ProductLookupPick | null>(null);
  const errorKind = ref<"provider" | "ai" | "ai_unavailable" | null>(null);

  let abort: AbortController | null = null;
  let requestSequence = 0;

  const candidates = computed<ProductLookupPick[]>(() => {
    if (aiCandidate.value) {
      return [aiCandidate.value];
    }
    return (results.value ?? []).map(product => ({ product, aiGuess: false }));
  });

  const noResults = computed(
    () =>
      !searching.value &&
      !aiLoading.value &&
      !errorKind.value &&
      results.value !== null &&
      candidates.value.length === 0
  );

  // "Ask AI instead" is offered after any completed provider search (zero
  // results or as a secondary action alongside results) — but only when AI
  // is configured, and not when the shown candidate already is the AI guess.
  const showAskAi = computed(
    () =>
      aiConfigured.value &&
      !aiCandidate.value &&
      !aiLoading.value &&
      (results.value !== null || errorKind.value === "provider")
  );

  const errorMessage = computed(() => {
    switch (errorKind.value) {
      case "provider":
        return t("components.entity.product_lookup.error_provider");
      case "ai_unavailable":
        return t("components.entity.product_lookup.error_ai_unavailable");
      case "ai":
        return t("components.entity.product_lookup.error_ai");
      default:
        return "";
    }
  });

  watch(
    () => props.open,
    open => {
      if (open) {
        keyword.value = props.seed?.trim() ?? "";
        reset();
      } else {
        cancelInflight();
      }
    }
  );

  function reset() {
    cancelInflight();
    searching.value = false;
    aiLoading.value = false;
    results.value = null;
    aiCandidate.value = null;
    errorKind.value = null;
  }

  function cancelInflight() {
    requestSequence++;
    abort?.abort();
    abort = null;
  }

  function onOpenChange(open: boolean) {
    emit("update:open", open);
  }

  function brandOf(product: BarcodeProduct): string {
    return product.manufacturer || product.item.manufacturer || "";
  }

  async function search() {
    const kw = keyword.value.trim();
    if (!kw || searching.value || aiLoading.value) {
      return;
    }

    cancelInflight();
    const sequence = requestSequence;
    const controller = new AbortController();
    abort = controller;

    searching.value = true;
    errorKind.value = null;
    results.value = null;
    aiCandidate.value = null;

    try {
      const result = await api.products.searchFromKeyword(kw, controller.signal);
      if (controller.signal.aborted || sequence !== requestSequence) {
        return;
      }
      if (result.error) {
        // Provider failure (e.g. 502: every provider errored) — distinct
        // from a clean zero-result response (204/empty list).
        errorKind.value = "provider";
        return;
      }
      results.value = result.status === 204 || !Array.isArray(result.data) ? [] : result.data;
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        errorKind.value = "provider";
      }
    } finally {
      if (sequence === requestSequence) {
        searching.value = false;
        if (abort === controller) {
          abort = null;
        }
      }
    }
  }

  async function askAi() {
    const kw = keyword.value.trim();
    if (!kw || searching.value || aiLoading.value) {
      return;
    }

    cancelInflight();
    const sequence = requestSequence;
    const controller = new AbortController();
    abort = controller;

    aiLoading.value = true;
    errorKind.value = null;
    aiCandidate.value = null;

    try {
      const result = await api.actions.identifyFromKeyword(kw, controller.signal);
      if (controller.signal.aborted || sequence !== requestSequence) {
        return;
      }
      if (result.status === 503) {
        errorKind.value = "ai_unavailable";
        return;
      }
      if (result.error || !result.data?.product) {
        errorKind.value = "ai";
        return;
      }
      aiCandidate.value = { product: result.data.product, aiGuess: true };
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        errorKind.value = "ai";
      }
    } finally {
      if (sequence === requestSequence) {
        aiLoading.value = false;
        if (abort === controller) {
          abort = null;
        }
      }
    }
  }

  function pick(candidate: ProductLookupPick) {
    emit("pick", candidate);
    emit("update:open", false);
  }
</script>
