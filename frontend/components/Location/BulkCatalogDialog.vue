<template>
  <Dialog :dialog-id="DialogID.BulkCatalog" :before-close="guardBeforeClose">
    <DialogContent
      :class="'w-full md:max-w-xl lg:max-w-4xl'"
      @escape-key-down="guardClose"
      @pointer-down-outside="guardClose"
    >
      <DialogHeader>
        <DialogTitle>{{ $t("components.location.bulk_catalog.title", { name: containerName }) }}</DialogTitle>
      </DialogHeader>

      <!-- add-photo row -->
      <div class="flex items-center gap-2">
        <Button type="button" :disabled="analyzing" @click="photoInput?.click()">
          <MdiCameraOutline class="mr-1 size-4" />
          {{ $t("components.location.bulk_catalog.add_photo") }}
        </Button>
        <div v-if="analyzing" class="flex items-center gap-2 text-sm text-muted-foreground">
          <MdiLoading class="size-4 animate-spin" />
          <span>{{
            analyzingSlow
              ? $t("components.entity.create_modal.ai_loading_slow")
              : $t("components.entity.create_modal.ai_loading")
          }}</span>
          <Button type="button" variant="ghost" size="sm" @click="abort?.abort()">{{ $t("global.cancel") }}</Button>
        </div>
        <Badge v-if="candidates.length > 0" variant="secondary">{{
          $t("components.entity.create_modal.ai_badge")
        }}</Badge>
      </div>
      <input
        ref="photoInput"
        type="file"
        accept="image/*"
        capture="environment"
        class="hidden"
        @change="onPhotoSelected"
      />

      <p v-if="!analyzing && photos.length > 0 && candidates.length === 0" class="text-sm text-muted-foreground">
        {{ $t("components.location.bulk_catalog.no_items_found") }}
      </p>

      <!-- candidate list, grouped by photo -->
      <div class="max-h-[55vh] space-y-4 overflow-y-auto">
        <template v-for="(_, pIdx) in photos" :key="pIdx">
          <h4 class="text-sm font-medium text-muted-foreground">
            {{ $t("components.location.bulk_catalog.photo_group", { n: pIdx + 1 }) }}
          </h4>
          <div
            v-for="c in candidatesForPhoto(pIdx)"
            :key="c.key"
            class="rounded-lg border p-3"
            :class="{ 'opacity-60': c.status === 'created' }"
          >
            <div class="flex items-start gap-3">
              <Checkbox
                :model-value="c.checked"
                :disabled="c.status === 'created' || committing"
                @update:model-value="v => (c.checked = !!v)"
              />
              <div class="grid flex-1 gap-2 md:grid-cols-2">
                <FormTextField
                  v-model="c.name"
                  :label="$t('components.location.bulk_catalog.card_name')"
                  :max-length="255"
                  :disabled="c.status === 'created'"
                />
                <div class="flex gap-2">
                  <FormTextField
                    v-model="c.manufacturer"
                    :label="$t('components.entity.create_modal.entity_manufacturer')"
                    :max-length="255"
                    :disabled="c.status === 'created'"
                  />
                  <FormTextField
                    v-model="c.modelNumber"
                    :label="$t('components.entity.create_modal.entity_model_number')"
                    :max-length="255"
                    :disabled="c.status === 'created'"
                  />
                  <FormTextField
                    v-model.number="c.quantity"
                    type="number"
                    :label="$t('components.location.bulk_catalog.card_quantity')"
                    :disabled="c.status === 'created'"
                    class="w-24"
                  />
                </div>
                <FormTextArea
                  v-model="c.description"
                  :label="$t('components.location.bulk_catalog.card_description')"
                  :max-length="1000"
                  :disabled="c.status === 'created'"
                  class="md:col-span-2"
                />
                <div v-if="c.categoryHints.length > 0" class="flex flex-wrap items-center gap-1 md:col-span-2">
                  <span class="text-xs text-muted-foreground">{{
                    $t("components.entity.create_modal.ai_hints_label")
                  }}</span>
                  <Button
                    v-for="hint in c.categoryHints"
                    :key="hint"
                    type="button"
                    variant="outline"
                    size="sm"
                    :disabled="c.status === 'created'"
                    @click="applyHint(c, hint)"
                    >{{ hint }}</Button
                  >
                </div>
              </div>
            </div>
            <div class="mt-1 flex gap-2">
              <Badge v-if="c.possibleDuplicate" variant="outline">{{
                $t("components.location.bulk_catalog.possible_duplicate")
              }}</Badge>
              <Badge v-if="c.status === 'created'" variant="secondary">{{
                $t("components.location.bulk_catalog.created")
              }}</Badge>
              <template v-if="c.status === 'failed'">
                <Badge variant="destructive">{{ $t("components.location.bulk_catalog.failed") }}</Badge>
                <Button type="button" variant="outline" size="sm" @click="commitOne(c)">{{
                  $t("components.location.bulk_catalog.retry")
                }}</Button>
              </template>
            </div>
          </div>
        </template>
      </div>

      <DialogFooter>
        <Button type="button" :disabled="committing || checkedPending.length === 0" @click="commit">
          {{ $t("components.location.bulk_catalog.create_n", { n: checkedPending.length }) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import type { PointerDownOutsideEvent } from "reka-ui";
  import { DialogID } from "@/components/ui/dialog-provider/utils";
  import { useDialog } from "@/components/ui/dialog-provider";
  import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
  import { Button } from "@/components/ui/button";
  import { Badge } from "@/components/ui/badge";
  import { Checkbox } from "@/components/ui/checkbox";
  import { toast } from "@/components/ui/sonner";
  import { useConfirm } from "@/composables/use-confirm";
  import FormTextField from "@/components/Form/TextField.vue";
  import FormTextArea from "@/components/Form/TextArea.vue";
  import MdiCameraOutline from "~icons/mdi/camera-outline";
  import MdiLoading from "~icons/mdi/loading";
  import { useTagStore } from "~/stores/tags";
  import { useEntityTypeStore } from "~~/stores/entityTypes";
  import { matchHintToTag } from "~~/lib/ai/hints";
  import { AttachmentTypes } from "~~/lib/api/types/non-generated";
  import { appendCandidates, toEntityCreate, type ReviewCandidate } from "~~/lib/bulk-catalog/session";

  const { t } = useI18n();
  const { registerOpenDialogCallback, closeDialog } = useDialog();
  const confirm = useConfirm();
  const api = useUserApi();
  const tagStore = useTagStore();
  const entityTypeStore = useEntityTypeStore();

  const containerId = ref("");
  const containerName = ref("");
  const photoInput = ref<HTMLInputElement | null>(null);
  const photos = ref<File[]>([]);
  const candidates = ref<ReviewCandidate[]>([]);
  const analyzing = ref(false);
  const analyzingSlow = ref(false);
  const abort = ref<AbortController | null>(null);
  const committing = ref(false);
  const createdCount = ref(0);

  // Indices into `photos` that have already been uploaded as container
  // attachments. commit() is re-entrant (retried after a partial failure),
  // so this lets the upload loop skip photos that already succeeded instead
  // of re-uploading them as duplicate attachments. Failed uploads are NOT
  // added here, so they remain retryable on the next commit().
  const uploadedPhotoIdx = new Set<number>();

  // Applied-hint tag ids per candidate, keyed by candidate key. Nothing in the
  // template reads this directly (it's only consulted at commit time), so a
  // plain Map avoids taking on Vue's deep-reactivity overhead for no benefit.
  const tagIdsByCandidate = new Map<string, string[]>();

  function tagIdsFor(c: ReviewCandidate): string[] {
    return tagIdsByCandidate.get(c.key) ?? [];
  }

  function candidatesForPhoto(i: number) {
    return candidates.value.filter(c => c.photoIndex === i);
  }

  const checkedPending = computed(() => candidates.value.filter(c => c.checked && c.status !== "created"));

  onMounted(() => {
    const cleanup = registerOpenDialogCallback(DialogID.BulkCatalog, params => {
      // Reset ALL state at callback entry -- this dialog is a singleton
      // reused across containers, so nothing from a previous open (or a
      // previous container) should leak into this one.
      abort.value?.abort();
      containerId.value = params.containerId;
      containerName.value = params.containerName;
      if (photoInput.value) {
        photoInput.value.value = "";
      }
      photos.value = [];
      candidates.value = [];
      analyzing.value = false;
      analyzingSlow.value = false;
      abort.value = null;
      committing.value = false;
      createdCount.value = 0;
      tagIdsByCandidate.clear();
      uploadedPhotoIdx.clear();
    });

    onUnmounted(cleanup);
  });

  async function applyHint(c: ReviewCandidate, hint: string) {
    const existing = matchHintToTag(hint, tagStore.tags);
    if (existing) {
      const ids = tagIdsFor(c);
      if (!ids.includes(existing.id)) {
        tagIdsByCandidate.set(c.key, [...ids, existing.id]);
      }
    } else {
      const { error, data } = await api.tags.create({ name: hint.trim(), color: "", description: "", icon: "" });
      if (error) {
        toast.error(t("components.entity.create_modal.toast.ai_hint_tag_failed"));
        return;
      }
      tagIdsByCandidate.set(c.key, [...tagIdsFor(c), data.id]);
      await tagStore.refresh();
    }
    c.categoryHints = c.categoryHints.filter(h => h !== hint);
  }

  async function onPhotoSelected(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file || analyzing.value) {
      return;
    }

    analyzing.value = true;
    analyzingSlow.value = false;
    const slowTimer = setTimeout(() => {
      analyzingSlow.value = true;
    }, 10_000);
    abort.value = new AbortController();

    try {
      const { data, error } = await api.actions.analyzePhotoBulk(file, abort.value.signal);
      if (error || !data) {
        toast.error(t("components.location.bulk_catalog.analyze_failed"));
        return;
      }
      photos.value.push(file);
      candidates.value = appendCandidates(candidates.value, data.candidates, photos.value.length - 1);
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        toast.error(t("components.location.bulk_catalog.analyze_failed"));
      }
    } finally {
      clearTimeout(slowTimer);
      analyzing.value = false;
      analyzingSlow.value = false;
      abort.value = null;
    }
  }

  async function commitOne(c: ReviewCandidate) {
    c.status = "creating";
    const { data, error } = await api.items.create(
      toEntityCreate(c, containerId.value, entityTypeStore.itemTypes[0]?.id ?? "", tagIdsFor(c))
    );
    if (error || !data) {
      c.status = "failed";
      return;
    }
    c.status = "created";
    createdCount.value++;
  }

  async function commit() {
    if (committing.value || checkedPending.value.length === 0) {
      return;
    }
    committing.value = true;

    // Strictly sequential -- mirrors CreateModal's batch-create paths, which
    // avoid firing concurrent creates at sqlite.
    for (const c of checkedPending.value) {
      await commitOne(c);
    }

    // The photos document the container's contents at scan time, so each one
    // attaches to the container itself (once), not to the individual items
    // created from it. Items are already created at this point, so an upload
    // failure here is non-fatal -- it's toasted but doesn't block the summary.
    // commit() can be re-invoked after a partial failure (retryable failed
    // candidates), so skip photos already uploaded by a prior call -- only
    // the ones that actually failed get retried.
    for (let i = 0; i < photos.value.length; i++) {
      if (uploadedPhotoIdx.has(i)) {
        continue;
      }
      const photo = photos.value[i]!;
      const photoName = `contents-snapshot-${i + 1}.jpg`;
      const { error } = await api.items.attachments.add(
        containerId.value,
        photo,
        photoName,
        AttachmentTypes.Photo,
        false
      );
      if (error) {
        toast.error(t("components.entity.create_modal.toast.upload_failed", { photoName }));
      } else {
        uploadedPhotoIdx.add(i);
      }
    }

    const failedCount = candidates.value.filter(c => c.status === "failed").length;
    toast.success(
      t("components.location.bulk_catalog.created_summary", { created: createdCount.value, failed: failedCount })
    );

    committing.value = false;

    // checkedPending only still contains candidates that were checked and
    // didn't reach "created" -- i.e. the failed ones. Empty means every
    // checked candidate succeeded.
    if (checkedPending.value.length === 0) {
      closeDialog(DialogID.BulkCatalog, { created: createdCount.value });
    }
  }

  // Shared by both dismissal guards below: candidates that would be silently
  // discarded if the dialog closed right now.
  function pendingUnconfirmed() {
    return candidates.value.filter(c => c.status === "pending" && c.checked);
  }

  // Same confirmation prompt for every dismissal path, so "x" button, Escape,
  // and outside-click all ask the user the identical question.
  async function confirmDiscard(pending: ReviewCandidate[]): Promise<boolean> {
    const { isCanceled } = await confirm.open(
      t("components.location.bulk_catalog.discard_confirm", { n: pending.length })
    );
    return !isCanceled;
  }

  // Guards accidental dismissal via Escape / click-outside while there are
  // checked-but-not-yet-created candidates, so a stray click doesn't silently
  // discard a batch of reviewed items. These reka-ui events are cancelable,
  // so preventDefault() must happen synchronously (before the confirm await)
  // to stop the dismissal -- see DismissableLayer, which checks
  // event.defaultPrevented immediately after emitting.
  async function guardClose(event: KeyboardEvent | PointerDownOutsideEvent) {
    if (committing.value) {
      return;
    }
    const pending = pendingUnconfirmed();
    if (pending.length === 0) {
      return;
    }
    event.preventDefault();
    if (await confirmDiscard(pending)) {
      closeDialog(DialogID.BulkCatalog, undefined);
    }
  }

  // Consulted by the shared Dialog wrapper's `before-close` prop before ANY
  // close request reaches closeDialog -- this is what catches the dialog's
  // built-in "x" button, which (unlike Escape/outside-click) fires directly
  // through DialogRoot's onOpenChange with no cancelable event to intercept.
  // Escape/outside-click are already handled by guardClose above via
  // preventDefault, but this also runs for them (harmlessly redundant when
  // guardClose already blocked the event, since closeDialog is never reached
  // in that case) -- see task report for the full dismissal-path analysis.
  async function guardBeforeClose(): Promise<boolean> {
    if (committing.value) {
      return true;
    }
    const pending = pendingUnconfirmed();
    if (pending.length === 0) {
      return true;
    }
    return confirmDiscard(pending);
  }
</script>
