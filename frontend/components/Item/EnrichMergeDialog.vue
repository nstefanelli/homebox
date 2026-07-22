<template>
  <DialogRoot :open="open" @update:open="onOpenChange">
    <DialogContent class="w-full md:max-w-xl lg:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{{ $t("components.item.enrich.title", { name: item.name }) }}</DialogTitle>
        <DialogDescription class="sr-only">
          {{ $t("components.item.enrich.title", { name: item.name }) }}
        </DialogDescription>
      </DialogHeader>

      <Badge v-if="aiGuess" variant="secondary" class="self-start">
        {{ $t("components.entity.create_modal.ai_badge") }}
      </Badge>

      <p v-if="rows.length === 0" class="text-sm text-muted-foreground">
        {{ $t("components.item.enrich.nothing_to_apply") }}
      </p>

      <div v-else class="flex max-h-[55vh] flex-col gap-2 overflow-y-auto">
        <label v-for="row in rows" :key="row.field" class="flex cursor-pointer items-start gap-3 rounded-lg border p-3">
          <Checkbox
            :model-value="row.checked"
            :disabled="applying"
            class="mt-0.5"
            @update:model-value="v => (row.checked = !!v)"
          />
          <div class="min-w-0 grow">
            <p class="text-sm font-medium">{{ fieldLabel(row.field) }}</p>

            <template v-if="row.kind === 'text'">
              <div class="mt-1 flex flex-wrap items-center gap-2 text-sm">
                <span class="text-muted-foreground" :class="{ italic: !row.current }">
                  {{ row.current || $t("components.item.enrich.empty_value") }}
                </span>
                <MdiArrowRight class="size-4 shrink-0 text-muted-foreground" />
                <span class="min-w-0 break-words">{{ row.proposed }}</span>
              </div>
            </template>

            <template v-else>
              <div class="mt-1 flex items-center gap-3">
                <img
                  :src="product.imageBase64"
                  class="size-16 rounded object-contain shadow-sm"
                  :alt="$t('components.item.enrich.photo_label')"
                />
                <span class="text-sm text-muted-foreground">
                  {{
                    row.primary
                      ? $t("components.item.enrich.photo_primary")
                      : $t("components.item.enrich.photo_additional")
                  }}
                </span>
              </div>
            </template>
          </div>
        </label>
      </div>

      <DialogFooter>
        <Button type="button" :disabled="applying || checkedRows.length === 0" @click="apply()">
          <MdiLoading v-if="applying" class="animate-spin" />
          {{ $t("components.item.enrich.apply") }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </DialogRoot>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { DialogRoot } from "reka-ui";
  import { toast } from "@/components/ui/sonner";
  import { Button } from "~/components/ui/button";
  import { Badge } from "~/components/ui/badge";
  import { Checkbox } from "@/components/ui/checkbox";
  import { DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
  import MdiArrowRight from "~icons/mdi/arrow-right";
  import MdiLoading from "~icons/mdi/loading";
  import type { BarcodeProduct, EntityOut } from "~~/lib/api/types/data-contracts";
  import { AttachmentTypes } from "~~/lib/api/types/non-generated";
  import { dataURLtoFile } from "~/components/Form/photo-uploader";
  import { computeMergePlan, proposedFromProduct, type MergeRow } from "~~/lib/enrich";

  /**
   * Per-field merge preview for "Enrich from lookup": one row per field
   * (current -> proposed) with a checkbox, defaults from the pure
   * computeMergePlan in lib/enrich.ts (blanks pre-checked, filled fields
   * opt-in). Apply commits the checked text fields in a single entity
   * update (full-entity spread + overrides, the same pattern the edit page
   * and renameToAssetId use) plus an optional photo attach via the same
   * attachment call the barcode create path uses.
   */

  const props = defineProps<{
    open: boolean;
    item: EntityOut;
    product: BarcodeProduct;
    aiGuess: boolean;
  }>();

  const emit = defineEmits<{
    "update:open": [value: boolean];
    /** Fired after at least one field/photo was applied successfully. */
    applied: [];
  }>();

  const { t } = useI18n();
  const api = useUserApi();

  const rows = ref<MergeRow[]>([]);
  const applying = ref(false);

  const checkedRows = computed(() => rows.value.filter(r => r.checked));

  watch(
    () => props.open,
    open => {
      if (open) {
        rows.value = computeMergePlan(
          {
            name: props.item.name ?? "",
            manufacturer: props.item.manufacturer ?? "",
            modelNumber: props.item.modelNumber ?? "",
            description: props.item.description ?? "",
            hasPhoto: props.item.attachments?.some(a => a.type === AttachmentTypes.Photo) ?? false,
          },
          proposedFromProduct(props.product)
        );
      }
    }
  );

  function onOpenChange(open: boolean) {
    if (applying.value) {
      return;
    }
    emit("update:open", open);
  }

  function fieldLabel(field: MergeRow["field"]): string {
    switch (field) {
      case "name":
        return t("items.name");
      case "manufacturer":
        return t("items.manufacturer");
      case "modelNumber":
        return t("items.model_number");
      case "description":
        return t("items.description");
      case "photo":
        return t("components.item.enrich.photo_label");
    }
  }

  async function apply() {
    if (applying.value || checkedRows.value.length === 0) {
      return;
    }

    applying.value = true;
    const appliedLabels: string[] = [];

    try {
      const textRows = checkedRows.value.filter((r): r is Extract<MergeRow, { kind: "text" }> => r.kind === "text");
      if (textRows.length > 0) {
        // Single entity update: spread the full entity, override only the
        // checked fields — unchecked fields go back byte-identical.
        const overrides: Partial<Record<"name" | "manufacturer" | "modelNumber" | "description", string>> = {};
        for (const row of textRows) {
          overrides[row.field] = row.proposed;
        }

        const { error } = await api.items.update(props.item.id, {
          ...props.item,
          ...overrides,
          parentId: props.item.parent?.id ?? null,
          entityTypeId: props.item.entityType?.id,
          tagIds: props.item.tags.map(tag => tag.id),
        });

        if (error) {
          toast.error(t("components.item.enrich.toast.update_failed"));
          return;
        }

        appliedLabels.push(...textRows.map(row => fieldLabel(row.field)));
      }

      const photoRow = checkedRows.value.find((r): r is Extract<MergeRow, { kind: "photo" }> => r.kind === "photo");
      if (photoRow) {
        // Same attach path the barcode-prefilled create flow uses: the
        // provider-fetched imageBase64 becomes a photo attachment.
        let attachFailed = false;
        try {
          const file = dataURLtoFile(props.product.imageBase64, "product_view.jpg");
          const { error } = await api.items.attachments.add(
            props.item.id,
            file,
            "product_view.jpg",
            AttachmentTypes.Photo,
            photoRow.primary
          );
          attachFailed = !!error;
        } catch {
          attachFailed = true;
        }

        if (attachFailed) {
          toast.error(t("components.item.enrich.toast.photo_failed"));
          if (appliedLabels.length === 0) {
            return;
          }
        } else {
          appliedLabels.push(fieldLabel("photo"));
        }
      }

      if (appliedLabels.length > 0) {
        toast.success(t("components.item.enrich.toast.applied", { fields: appliedLabels.join(", ") }));
        emit("applied");
        emit("update:open", false);
      }
    } catch {
      toast.error(t("components.item.enrich.toast.update_failed"));
    } finally {
      applying.value = false;
    }
  }
</script>
