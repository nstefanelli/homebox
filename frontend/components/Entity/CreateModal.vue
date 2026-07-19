<template>
  <BaseModal :dialog-id="DialogID.CreateEntity">
    <template #title>
      <div class="flex items-center gap-2 text-nowrap">
        <span>Create</span>
        <EntitySelector
          :selected-entity-type="selectedEntityType?.id"
          :entity-types="subItemCreate ? entityTypes.filter(t => !t.isLocation) : entityTypes"
          size="sm"
          @entity-type-changed="onEntityTypeChanged"
        />
      </div>
    </template>
    <template #header-actions>
      <div class="flex gap-2">
        <TooltipProvider :delay-duration="0">
          <!-- Template selector button -->
          <Tooltip v-if="!selectedEntityType?.isLocation || selectedEntityType?.isContainer">
            <TooltipTrigger>
              <TemplateSelector v-model="selectedTemplate" compact @template-selected="handleTemplateSelected" />
            </TooltipTrigger>
            <TooltipContent>
              <p>{{ $t("components.template.apply_template") }}</p>
            </TooltipContent>
          </Tooltip>

          <ButtonGroup>
            <Tooltip>
              <TooltipTrigger>
                <Button variant="outline" :disabled="loading" size="icon" data-pos="start" @click="openQrScannerPage()">
                  <MdiBarcodeScan class="size-5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>
                  {{ $t("components.entity.create_modal.product_tooltip_scan_barcode") }}
                </p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger>
                <Button
                  variant="outline"
                  :disabled="loading"
                  size="icon"
                  :data-pos="aiPhotoEnabled ? undefined : 'end'"
                  @click="openBarcodeDialog()"
                >
                  <MdiBarcode class="size-5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>
                  {{ $t("components.entity.create_modal.product_tooltip_input_barcode") }}
                </p>
              </TooltipContent>
            </Tooltip>
            <Tooltip v-if="aiPhotoEnabled">
              <TooltipTrigger>
                <Button
                  variant="outline"
                  :disabled="loading || aiLoading"
                  size="icon"
                  data-pos="end"
                  @click="openAiPhotoPicker()"
                >
                  <MdiCameraOutline class="size-5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>
                  {{ $t("components.entity.create_modal.product_tooltip_ai_photo") }}
                </p>
              </TooltipContent>
            </Tooltip>
          </ButtonGroup>
        </TooltipProvider>
      </div>
    </template>

    <input
      ref="aiPhotoInput"
      type="file"
      accept="image/*"
      capture="environment"
      class="hidden"
      @change="onAiPhotoSelected"
    />

    <form class="flex min-w-0 flex-col gap-2" @submit.prevent="create()">
      <LocationSelector v-model="form.location" />

      <div v-if="aiLoading" class="flex items-center gap-2 text-sm text-muted-foreground">
        <MdiLoading class="size-4 animate-spin" />
        <span>{{
          aiLoadingSlow
            ? $t("components.entity.create_modal.ai_loading_slow")
            : $t("components.entity.create_modal.ai_loading")
        }}</span>
        <Button type="button" variant="ghost" size="sm" @click="cancelAiAnalyze()">
          {{ $t("global.cancel") }}
        </Button>
      </div>
      <Badge v-if="aiPrefill" variant="secondary" class="self-start">
        {{ $t("components.entity.create_modal.ai_badge") }}
      </Badge>

      <!-- Template Info Display - Collapsible banner with distinct styling -->
      <div v-if="templateData" class="rounded-lg border-l-4 border-l-primary bg-primary/5 p-3">
        <div class="flex items-start justify-between gap-2">
          <div class="flex flex-1 items-start gap-2">
            <MdiFileDocumentOutline class="mt-0.5 size-4 shrink-0 text-primary" />
            <div class="flex-1">
              <h4 class="text-sm font-medium text-foreground">
                {{
                  $t("components.template.using_template", {
                    name: templateData.name,
                  })
                }}
              </h4>
              <button
                type="button"
                class="mt-1 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                @click="showTemplateDetails = !showTemplateDetails"
              >
                <span v-if="!showTemplateDetails">{{ $t("components.template.show_defaults") }}</span>
                <span v-else>{{ $t("components.template.hide_defaults") }}</span>
                <MdiChevronDown class="size-4 transition-transform" :class="{ 'rotate-180': showTemplateDetails }" />
              </button>
            </div>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            class="size-7 shrink-0"
            :aria-label="$t('components.entity.create_modal.clear_template')"
            @click="clearTemplate"
          >
            <MdiClose class="size-4" />
          </Button>
        </div>

        <!-- Collapsible details section -->
        <div v-if="showTemplateDetails" class="mt-3 border-t border-primary/20 pt-3">
          <div class="flex flex-col gap-2 text-xs text-muted-foreground">
            <p v-if="templateData.description" class="text-foreground/80">
              {{ templateData.description }}
            </p>
            <div class="grid grid-cols-2 gap-x-4 gap-y-1">
              <div v-if="templateData.defaultName">
                <span class="font-medium">{{ $t("global.name") }}:</span>
                {{ templateData.defaultName }}
              </div>
              <div>
                <span class="font-medium">{{ $t("global.quantity") }}:</span>
                {{ templateData.defaultQuantity }}
              </div>
              <div>
                <span class="font-medium">{{ $t("global.insured") }}:</span>
                {{ templateData.defaultInsured ? $t("global.yes") : $t("global.no") }}
              </div>
              <div v-if="templateData.defaultManufacturer">
                <span class="font-medium">{{ $t("components.template.form.manufacturer") }}:</span>
                {{ templateData.defaultManufacturer }}
              </div>
              <div v-if="templateData.defaultModelNumber">
                <span class="font-medium">{{ $t("components.template.form.model_number") }}:</span>
                {{ templateData.defaultModelNumber }}
              </div>
              <div v-if="templateData.defaultLifetimeWarranty">
                <span class="font-medium">{{ $t("components.template.form.lifetime_warranty") }}:</span>
                {{ $t("global.yes") }}
              </div>
              <div v-if="templateData.defaultLocation">
                <span class="font-medium">{{ $t("components.template.form.location") }}:</span>
                {{ templateData.defaultLocation.name }}
              </div>
            </div>
            <div v-if="templateData.defaultTags && templateData.defaultTags.length > 0" class="mt-1">
              <span class="font-medium">{{ $t("global.tags") }}:</span>
              {{ templateData.defaultTags.map((t: any) => t.name).join(", ") }}
            </div>
            <div v-if="templateData.defaultDescription" class="mt-1">
              <p class="font-medium">{{ $t("components.template.form.item_description") }}:</p>
              <p class="ml-2">{{ templateData.defaultDescription }}</p>
            </div>
            <div v-if="templateData.fields && templateData.fields.length > 0" class="mt-1">
              <p class="font-medium">{{ $t("components.template.form.custom_fields") }}:</p>
              <ul class="ml-4 flex list-none flex-col gap-1">
                <li v-for="field in templateData.fields" :key="field.id">
                  <span class="font-medium">{{ field.name }}:</span>
                  <span> {{ field.textValue || $t("components.template.empty_value") }}</span>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <ItemSelector
        v-if="subItemCreate"
        v-model="parent"
        v-model:search="query"
        :label="$t('components.entity.create_modal.parent_item')"
        :items="results"
        item-text="name"
        :no-results-text="$t('components.entity.create_modal.item_selector_no_results_text')"
        :is-loading="isLoading"
        :trigger-search="triggerSearch"
      >
        <template #display="{ item }">
          <span v-if="item && typeof item === 'object'" class="flex items-center gap-2">
            <component
              :is="
                resolveEntityIcon({
                  icon: asEntitySummary(item).icon,
                  typeIcon: asEntitySummary(item).entityType?.icon,
                  isContainer: asEntitySummary(item).entityType?.isContainer,
                  isLocation: true,
                })
              "
              v-if="asEntitySummary(item).entityType?.isLocation"
              class="size-4 shrink-0"
            />
            {{ asEntitySummary(item).name }}
          </span>
          <template v-else>
            <!--
              Reproduces ItemSelector's own default fallback for the no-selection /
              cleared-selection states (`displayValue(value) || localizedPlaceholder`
              in components/Item/Selector.vue), since providing a #display slot at all
              suppresses that default for BOTH the trigger button (item = "" or null)
              and each CommandItem row (item = EntitySummary, handled above).
            -->
            {{ (typeof item === "string" ? item : "") || $t("components.item.selector.placeholder") }}
          </template>
        </template>
      </ItemSelector>
      <FormTextField
        ref="nameInput"
        v-model="form.name"
        :trigger-focus="focused"
        :autofocus="true"
        :label="
          $t('components.entity.create_modal.entity_name', {
            type: selectedEntityType?.name || '',
          })
        "
        :max-length="255"
        :min-length="1"
      />
      <FormTextField
        v-if="!selectedEntityType?.isLocation"
        v-model.number="form.quantity"
        :label="
          $t('components.entity.create_modal.entity_quantity', {
            type: selectedEntityType?.name || t('global.entity'),
          })
        "
        type="number"
        step="any"
        :min="0"
      />
      <FormTextField
        v-if="selectedEntityType?.isContainer"
        v-model.number="form.count"
        type="number"
        :min="1"
        :max="100"
        :step="1"
        :label="$t('components.entity.create_modal.container_count')"
      />
      <FormTextArea
        v-model="form.description"
        :label="
          $t('components.entity.create_modal.entity_description', {
            type: selectedEntityType?.name || t('global.entity'),
          })
        "
        :max-length="1000"
      />
      <FormTextField
        v-if="!selectedEntityType?.isLocation"
        v-model="form.manufacturer"
        :label="$t('components.entity.create_modal.entity_manufacturer')"
        :max-length="255"
      />
      <FormTextField
        v-if="!selectedEntityType?.isLocation"
        v-model="form.modelNumber"
        :label="$t('components.entity.create_modal.entity_model_number')"
        :max-length="255"
      />
      <TagSelector v-model="form.tags" :tags="tags ?? []" />
      <IconSelector
        v-if="selectedEntityType?.isLocation"
        v-model="form.icon"
        :label="$t('components.entity.create_modal.entity_icon')"
      />
      <div v-if="categoryHints.length > 0" class="flex flex-wrap items-center gap-1">
        <span class="text-xs text-muted-foreground">
          {{ $t("components.entity.create_modal.ai_hints_label") }}
        </span>
        <Button
          v-for="hint in categoryHints"
          :key="hint"
          type="button"
          variant="outline"
          size="sm"
          @click="applyHint(hint)"
        >
          {{ hint }}
        </Button>
      </div>
      <PhotoUploader
        :label="
          $t('components.entity.create_modal.entity_photo', {
            type: selectedEntityType?.name || t('global.entity'),
          })
        "
        :button-label="$t('components.entity.create_modal.upload_photos')"
        :existing-count="form.photos.length"
        @selected="appendPhotos"
      />
      <div class="mt-4 flex flex-row-reverse">
        <ButtonGroup>
          <Button :disabled="loading" type="submit" class="group">
            <div class="relative mx-2">
              <div
                class="absolute inset-0 flex items-center justify-center transition-transform duration-300 group-hover:rotate-[360deg]"
              >
                <MdiPackageVariant class="size-5 group-hover:hidden" />
                <MdiPackageVariantClosed class="hidden size-5 group-hover:block" />
              </div>
            </div>
            {{ $t("global.create") }}
          </Button>
          <Button variant="outline" :disabled="loading" type="button" @click="create(false)">
            {{ $t("global.create_and_add") }}
          </Button>
        </ButtonGroup>
      </div>

      <PhotoUploaderPreview
        :photos="form.photos"
        @delete="deletePhotoAt"
        @rotate="rotatePhotoAt"
        @set-primary="setPrimaryPhotoAt"
      />
    </form>
  </BaseModal>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { DialogID } from "@/components/ui/dialog-provider/utils";
  import { toast } from "@/components/ui/sonner";
  import { Button, ButtonGroup } from "~/components/ui/button";
  import BaseModal from "@/components/App/CreateModal.vue";
  import type {
    BarcodeProduct,
    EntityCreate,
    EntitySummary,
    EntityTemplateOut,
    EntityTemplateSummary,
    EntityOut,
    EntityTypeSummary,
  } from "~~/lib/api/types/data-contracts";
  import { useTagStore } from "~/stores/tags";
  import { useLocationStore } from "~~/stores/locations";
  import { useLabelPrintQueue } from "~~/stores/labels";
  import MdiBarcode from "~icons/mdi/barcode";
  import MdiBarcodeScan from "~icons/mdi/barcode-scan";
  import MdiPackageVariant from "~icons/mdi/package-variant";
  import MdiPackageVariantClosed from "~icons/mdi/package-variant-closed";
  import MdiFileDocumentOutline from "~icons/mdi/file-document-outline";
  import MdiChevronDown from "~icons/mdi/chevron-down";
  import MdiClose from "~icons/mdi/close";
  import MdiCameraOutline from "~icons/mdi/camera-outline";
  import MdiLoading from "~icons/mdi/loading";
  import { Badge } from "~/components/ui/badge";
  import { detectProductBarcode } from "~~/lib/barcode/from-file";
  import { resolveEntityIcon } from "~~/lib/icons";
  import { matchHintToTag } from "~~/lib/ai/hints";
  import { AttachmentTypes } from "~~/lib/api/types/non-generated";
  import { useDialog, useDialogHotkey } from "~/components/ui/dialog-provider";
  import TagSelector from "~/components/Tag/Selector.vue";
  import IconSelector from "@/components/Form/IconSelector.vue";
  import ItemSelector from "~/components/Item/Selector.vue";
  import TemplateSelector from "~/components/Template/Selector.vue";
  import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "~/components/ui/tooltip";
  import LocationSelector from "~/components/Location/Selector.vue";
  import FormTextField from "~/components/Form/TextField.vue";
  import FormTextArea from "~/components/Form/TextArea.vue";
  import PhotoUploader from "~/components/Form/PhotoUploader.vue";
  import PhotoUploaderPreview from "~/components/Form/PhotoUploaderPreview.vue";
  import {
    deletePhoto,
    dataURLtoFile,
    rotatePhotoPreview,
    setPrimaryPhoto,
    type PhotoPreview,
  } from "~/components/Form/photo-uploader";
  import { useEntityTypeStore } from "~~/stores/entityTypes";
  import { useIntegrationsStore } from "~~/stores/integrations";
  import EntitySelector from "~/components/Entity/Selector.vue";

  const { t } = useI18n();
  const { openDialog, closeDialog, registerOpenDialogCallback, activeDialog } = useDialog();

  useDialogHotkey(DialogID.CreateEntity, { code: "Digit1", shift: true }, () => ({
    baseType: "item",
  }));
  useDialogHotkey(DialogID.CreateEntity, { code: "Digit2", shift: true }, () => ({
    baseType: "location",
  }));

  const entityTypeStore = useEntityTypeStore();

  const api = useUserApi();

  const integrationsStore = useIntegrationsStore();
  void integrationsStore.ensureFetched().catch(() => {
    // AI controls remain hidden when the background capability check fails.
  });
  const aiPhotoEnabled = computed(() => integrationsStore.aiConfigured);

  const aiPhotoInput = ref<HTMLInputElement | null>(null);
  const aiLoading = ref(false);
  const aiLoadingSlow = ref(false);
  const aiPrefill = ref(false);
  const categoryHints = ref<string[]>([]);
  let aiAbort: AbortController | null = null;
  let aiRequestSequence = 0;

  const locationsStore = useLocationStore();
  const locations = computed(() => locationsStore.allLocations);

  const tagStore = useTagStore();
  const tags = computed(() => tagStore.tags);

  const route = useRoute();

  const parent = ref();
  const { query, results, isLoading, triggerSearch } = useItemSearch(api, {
    immediate: false,
  });
  const subItemCreate = ref();

  // ItemSelector's #display slot types `item` as `string | ItemsObject` (its generic prop
  // shape); the parent-item search results are always EntitySummary in practice, so narrow
  // via `unknown` (a direct cast doesn't type-check — the two types don't sufficiently overlap).
  function asEntitySummary(item: unknown): EntitySummary {
    return item as unknown as EntitySummary;
  }

  const tagId = computed(() => {
    if (route.fullPath.includes("/tag/")) {
      return route.params.id;
    }
    return null;
  });

  const locationId = computed(() => {
    if (route.fullPath.includes("/location/")) {
      return route.params.id;
    }
    return null;
  });

  const itemId = computed(() => {
    if (route.fullPath.includes("/item/")) {
      return route.params.id;
    }
    return null;
  });

  const nameInput = ref<HTMLInputElement | null>(null);

  // Entity type selection
  const entityTypes = computed(() => entityTypeStore.allTypes);
  const selectedEntityType = ref<EntityTypeSummary | null>(null);
  let templateLoadSequence = 0;

  async function onEntityTypeChanged(typeId: string) {
    const loadSequence = ++templateLoadSequence;
    const et = entityTypes.value.find(t => t.id === typeId);
    selectedEntityType.value = et || null;

    // A template the user picked explicitly takes precedence over the entity
    // type's default template, so don't overwrite it when the type changes.
    // Containers can use templates too, so a user-selected template also
    // survives a switch into a container type. Plain (non-container)
    // locations don't use templates, so they still clear it below.
    if (templateUserSelected.value && (!et?.isLocation || et?.isContainer)) {
      return;
    }

    // Item types and container-location types both support defaults. Plain
    // location types do not.
    if ((et?.isLocation && !et.isContainer) || !et?.defaultTemplateId || !et.defaultTemplate) {
      clearTemplate();
    } else {
      let result;
      try {
        result = await api.templates.get(et.defaultTemplateId);
      } catch {
        toast.error(t("components.template.toast.load_failed"));
        return;
      }
      const { data, error } = result;
      if (loadSequence !== templateLoadSequence || selectedEntityType.value?.id !== typeId) {
        return;
      }
      if (!error && data) {
        selectedTemplate.value = {
          id: data.id,
          name: data.name,
          description: data.description,
        } as EntityTemplateSummary;
        templateData.value = data;
        form.quantity = data.defaultQuantity;
        if (data.defaultName) form.name = data.defaultName;
        if (data.defaultDescription) form.description = data.defaultDescription;
        if (data.defaultLocation) {
          const found = locations.value.find(l => l.id === data.defaultLocation!.id);
          if (found) form.location = found;
        }
        if (data.defaultTags && data.defaultTags.length > 0) {
          form.tags = data.defaultTags.map(l => l.id);
        }
        toast.success(t("components.template.toast.applied", { name: data.name }));
      }
    }
  }

  const LAST_TEMPLATE_KEY = "homebox:lastUsedTemplate";

  const loading = ref(false);
  const focused = ref(false);
  const selectedTemplate = ref<EntityTemplateSummary | null>(null);
  const templateData = ref<EntityTemplateOut | null>(null);
  // Tracks whether the current template was chosen explicitly by the user (vs.
  // auto-applied from an entity type's default template). User selections win.
  const templateUserSelected = ref(false);
  const showTemplateDetails = ref(false);
  const form = reactive({
    location: locations.value && locations.value.length > 0 ? locations.value[0] : ({} as EntityOut),
    parentId: null,
    name: "",
    quantity: 1,
    count: 1,
    description: "",
    manufacturer: "",
    modelNumber: "",
    icon: "",
    color: "",
    tags: [] as string[],
    photos: [] as PhotoPreview[],
  });

  async function handleTemplateSelected(template: EntityTemplateSummary | null) {
    const loadSequence = ++templateLoadSequence;
    if (!template) {
      // Template was deselected, clear template data and remove from storage
      templateData.value = null;
      templateUserSelected.value = false;
      form.quantity = 1;
      localStorage.removeItem(LAST_TEMPLATE_KEY);
      return;
    }

    templateUserSelected.value = true;

    // Load full template details
    let result;
    try {
      result = await api.templates.get(template.id);
    } catch {
      toast.error(t("components.template.toast.load_failed"));
      return;
    }
    const { data, error } = result;
    if (loadSequence !== templateLoadSequence || selectedTemplate.value?.id !== template.id) {
      return;
    }
    if (error || !data) {
      toast.error(t("components.template.toast.load_failed"));
      return;
    }

    // Store template data for display and item creation
    templateData.value = data;

    // Pre-fill form with template defaults
    form.quantity = data.defaultQuantity;
    if (data.defaultName) {
      form.name = data.defaultName;
    }
    if (data.defaultDescription) {
      form.description = data.defaultDescription;
    }
    // Pre-fill location if template has one and current form doesn't
    if (data.defaultLocation && !form.location?.id) {
      const found = locations.value.find(l => l.id === data.defaultLocation!.id);
      if (found) {
        form.location = found;
      }
    }
    // Pre-fill tags from template
    if (data.defaultTags && data.defaultTags.length > 0) {
      form.tags = data.defaultTags.map(l => l.id);
    }

    // Save template ID to localStorage for persistence
    localStorage.setItem(LAST_TEMPLATE_KEY, template.id);

    toast.success(t("components.template.toast.applied", { name: data.name }));
  }

  async function restoreLastTemplate() {
    const loadSequence = ++templateLoadSequence;
    const lastTemplateId = localStorage.getItem(LAST_TEMPLATE_KEY);
    if (!lastTemplateId) return;

    // Load the template details
    let result;
    try {
      result = await api.templates.get(lastTemplateId);
    } catch {
      localStorage.removeItem(LAST_TEMPLATE_KEY);
      return;
    }
    const { data, error } = result;
    if (loadSequence !== templateLoadSequence) {
      return;
    }
    if (error || !data) {
      // Template might have been deleted, clear the stored ID
      localStorage.removeItem(LAST_TEMPLATE_KEY);
      return;
    }

    // Set the template. A restored template reflects the user's last explicit
    // choice, so treat it as user-selected for override purposes.
    selectedTemplate.value = {
      id: data.id,
      name: data.name,
      description: data.description,
    } as EntityTemplateSummary;
    templateData.value = data;
    templateUserSelected.value = true;
    form.quantity = data.defaultQuantity;
    if (data.defaultName) {
      form.name = data.defaultName;
    }
    if (data.defaultDescription) {
      form.description = data.defaultDescription;
    }
    // Pre-fill location if template has one
    if (data.defaultLocation) {
      const found = locations.value.find(l => l.id === data.defaultLocation!.id);
      if (found) {
        form.location = found;
      }
    }
    // Pre-fill tags from template
    if (data.defaultTags && data.defaultTags.length > 0) {
      form.tags = data.defaultTags.map(l => l.id);
    }
  }

  function clearTemplate() {
    templateLoadSequence++;
    selectedTemplate.value = null;
    templateData.value = null;
    templateUserSelected.value = false;
    showTemplateDetails.value = false;
    form.quantity = 1;
    localStorage.removeItem(LAST_TEMPLATE_KEY);
  }

  watch(
    parent,
    newParent => {
      if (newParent && newParent.id && subItemCreate.value) {
        form.parentId = newParent.id;
      } else {
        form.parentId = null;
      }
    },
    { immediate: true }
  );

  const { shift } = useMagicKeys();
  function appendPhotos(photos: PhotoPreview[]) {
    form.photos.push(...photos);
  }

  function deletePhotoAt(index: number) {
    form.photos = deletePhoto(form.photos, index);
  }

  function setPrimaryPhotoAt(index: number) {
    form.photos = setPrimaryPhoto(form.photos, index);
  }

  async function rotatePhotoAt(index: number) {
    const photo = form.photos[index];
    if (!photo) return;

    try {
      form.photos[index] = await rotatePhotoPreview(photo);
    } catch {
      toast.error(t("components.entity.create_modal.toast.rotate_process_failed"));
    }
  }

  function applyProductPrefill(product: BarcodeProduct) {
    // A product prefill (AI or barcode) carries manufacturer/modelNumber, but the
    // template creation DTO (api.templates.createItem's request) has no such
    // fields -- so an active template would silently swallow them when create()
    // routes through the template path instead of the plain-item path. Product
    // prefill wins: clear any active template so create() takes the plain-item
    // path that carries manufacturer/model. clearTemplate() only touches
    // template state + form.quantity, so it's safe regardless of ordering
    // relative to the field assignments below.
    if (templateData.value) {
      clearTemplate();
    }

    form.name = product.item.name;
    form.description = product.item.description;
    form.manufacturer = product.manufacturer || product.item.manufacturer || "";
    form.modelNumber = product.modelNumber || product.item.modelNumber || "";

    if (product.imageURL) {
      appendPhotos([
        {
          photoName: "product_view.jpg",
          fileBase64: product.imageBase64,
          primary: form.photos.length === 0,
          file: dataURLtoFile(product.imageBase64, "product_view.jpg"),
        },
      ]);
    }
  }

  function openAiPhotoPicker() {
    aiPhotoInput.value?.click();
  }

  function cancelAiAnalyze() {
    aiRequestSequence++;
    aiAbort?.abort();
    aiAbort = null;
    aiLoading.value = false;
    aiLoadingSlow.value = false;
  }

  async function applyHint(hint: string) {
    const existing = matchHintToTag(hint, tags.value);
    if (existing) {
      if (!form.tags.includes(existing.id)) {
        form.tags = [...form.tags, existing.id];
      }
    } else {
      let result;
      try {
        result = await api.tags.create({
          name: hint.trim(),
          color: "",
          description: "",
          icon: "",
        });
      } catch {
        toast.error(t("components.entity.create_modal.toast.ai_hint_tag_failed"));
        return;
      }
      const { error, data } = result;
      if (error) {
        toast.error(t("components.entity.create_modal.toast.ai_hint_tag_failed"));
        return;
      }
      form.tags = [...form.tags, data.id];
      try {
        await tagStore.refresh();
      } catch {
        // The created tag is already attached by id. A later store refresh
        // will make it available in selectors.
      }
    }
    categoryHints.value = categoryHints.value.filter(h => h !== hint);
  }

  async function onAiPhotoSelected(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file || aiLoading.value) {
      return;
    }

    aiLoading.value = true;
    aiLoadingSlow.value = false;
    aiPrefill.value = false;
    categoryHints.value = [];
    const requestSequence = ++aiRequestSequence;
    const controller = new AbortController();
    aiAbort = controller;
    const slowTimer = setTimeout(() => {
      if (requestSequence === aiRequestSequence) {
        aiLoadingSlow.value = true;
      }
    }, 10_000);

    try {
      // Lane 1: barcode visible in the photo -> existing UPC pipeline, authoritative.
      const barcode = await detectProductBarcode(file);
      if (controller.signal.aborted || requestSequence !== aiRequestSequence) {
        return;
      }
      if (barcode) {
        const { data, error } = await api.products.searchFromBarcode(barcode, controller.signal);
        if (controller.signal.aborted || requestSequence !== aiRequestSequence) {
          return;
        }
        if (!error && data && data.length > 0) {
          applyProductPrefill(data[0]!);
          return;
        }
        // UPC miss: fall through to the vision lane with the same photo.
      }

      // Lane 2: vision analysis.
      const { data, error } = await api.actions.analyzePhoto(file, controller.signal);
      if (controller.signal.aborted || requestSequence !== aiRequestSequence) {
        return;
      }
      if (error || !data || data.products.length === 0) {
        toast.error(t("components.entity.create_modal.toast.ai_failed"));
        return;
      }
      applyProductPrefill(data.products[0]!);
      aiPrefill.value = true;
      categoryHints.value = data.categoryHints ?? [];
    } catch (err) {
      if (!(err instanceof DOMException && err.name === "AbortError")) {
        toast.error(t("components.entity.create_modal.toast.ai_failed"));
      }
    } finally {
      clearTimeout(slowTimer);
      if (requestSequence === aiRequestSequence) {
        aiLoading.value = false;
        aiLoadingSlow.value = false;
        if (aiAbort === controller) {
          aiAbort = null;
        }
      }
    }
  }

  // The create dialog is a singleton and remains mounted while hidden. Abort
  // immediately on dismissal so a slow barcode decode / vision response
  // cannot prefill a later dialog session.
  watch(activeDialog, (current, previous) => {
    if (previous === DialogID.CreateEntity && current !== DialogID.CreateEntity) {
      cancelAiAnalyze();
    }
  });

  onMounted(() => {
    const cleanup = registerOpenDialogCallback(DialogID.CreateEntity, async params => {
      cancelAiAnalyze();
      subItemCreate.value = false;
      let parentItemLocationId = null;
      parent.value = {};
      form.parentId = null;
      form.manufacturer = "";
      form.modelNumber = "";
      form.icon = "";
      aiPrefill.value = false;
      categoryHints.value = [];

      if (params.baseType === "item") {
        selectedEntityType.value = entityTypes.value.find(t => !t.isLocation) || null;

        subItemCreate.value = params.subItem;

        if (subItemCreate.value && itemId.value) {
          const itemIdRead = typeof itemId.value === "string" ? (itemId.value as string) : itemId.value[0]!;
          let data: EntityOut | undefined;
          try {
            const result = await api.items.get(itemIdRead);
            data = result.data;
            if (result.error || !data) {
              toast.error(t("components.entity.create_modal.toast.failed_load_parent"));
            }
          } catch {
            toast.error(t("components.entity.create_modal.toast.failed_load_parent"));
          }

          if (data) {
            parent.value = data;
          }

          if (data?.parent) {
            const loc = data.parent;
            parentItemLocationId = loc.id;
          }
        }

        if (params.product) {
          applyProductPrefill(params.product);
        } else {
          // Restore last used template if available -- but only when there's no
          // product prefill, since a restored template would silently swallow
          // the prefilled manufacturer/modelNumber (see applyProductPrefill).
          await restoreLastTemplate();
        }
      } else {
        selectedEntityType.value = entityTypes.value.find(t => t.isLocation) || null;
      }

      const locId = locationId.value ? locationId.value : parentItemLocationId;

      if (locId) {
        const found = locations.value.find(l => l.id === locId);
        if (found) {
          form.location = found;
        }
      }

      if (tagId.value) {
        form.tags = tags.value.filter(l => l.id === tagId.value).map(l => l.id);
      }
    });

    onUnmounted(cleanup);
  });

  async function create(close = true) {
    // An empty entityTypeId serializes to "" and fails UUID unmarshalling on the
    // backend, so block creation up front rather than firing a doomed request.
    if (!selectedEntityType.value?.id) {
      toast.error(t("components.entity.create_modal.toast.please_select_entity_type"));
      return;
    }

    if (!form.name.trim()) {
      toast.error(t("components.entity.create_modal.toast.name_required"));
      return;
    }

    // Items must live somewhere, but a top-level location has no parent, so the
    // parent location selector is optional when creating a location.
    if (!selectedEntityType.value?.isLocation && !form.location?.id) {
      toast.error(t("components.entity.create_modal.toast.please_select_location"));
      return;
    }

    if (loading.value) {
      toast.error(
        t("components.entity.create_modal.toast.already_creating", {
          type: selectedEntityType.value?.name || t("global.entity"),
        })
      );
      return;
    }

    loading.value = true;

    if (shift?.value) close = false;

    const rawContainerCount = Number(form.count);
    const containerCount = Number.isFinite(rawContainerCount)
      ? Math.min(100, Math.max(1, Math.floor(rawContainerCount)))
      : 1;

    try {
      // Container + template: batch-create form.count containers from the
      // template in a single request. count:1 is just a batch of one, so this
      // also covers "template + quantity 1" -- no separate single-create path
      // needed for containers once a template is selected.
      if (selectedEntityType.value?.isContainer && templateData.value) {
        const { data: created, error } = await api.templates.batchCreate(templateData.value.id, {
          count: containerCount,
          namePrefix: form.name,
          startNumber: 0, // 0 = backend infers next number from existing "<prefix> NN" names
          parentId: (form.location?.id || null) as string,
          entityTypeId: selectedEntityType.value?.id || "",
          tagIds: form.tags,
        });

        if (error) {
          toast.error(t("components.entity.create_modal.batch_failed"));
          return;
        }

        toastBatchCreated(created);
        await uploadPhotosToBatch(created.map(e => e.id));

        form.name = "";
        form.quantity = 1;
        form.count = 1;
        form.description = "";
        form.manufacturer = "";
        form.modelNumber = "";
        form.icon = "";
        form.color = "";
        form.photos = [];
        form.tags = [];
        selectedTemplate.value = null;
        templateData.value = null;
        templateUserSelected.value = false;
        showTemplateDetails.value = false;
        aiPrefill.value = false;
        categoryHints.value = [];
        focused.value = false;
        loading.value = false;

        if (close && created[0]) {
          closeDialog(DialogID.CreateEntity);
          navigateTo(`/location/${created[0].id}`);
        } else if (!close) {
          await restoreLastTemplate();
        }
        return;
      }

      // Container, no template, count > 1 (e.g. a UPC-scanned tote where the
      // user just wants N more of the same bin): sequential numbered creates,
      // mirroring ItemChangeDetails' sequential-PATCH pattern (avoids sqlite
      // write contention from firing them all concurrently). Numbering
      // restarts at 01 per prefix since this client-side path has no
      // server-side inference -- the template batch path above remains the
      // numbering-aware one.
      if (selectedEntityType.value?.isContainer && !templateData.value && containerCount > 1) {
        const prefix = form.name;
        const created: EntityOut[] = [];

        for (let i = 1; i <= containerCount; i++) {
          const { data: createdOne, error } = await api.items.createLocation({
            name: `${prefix} ${String(i).padStart(2, "0")}`,
            description: form.description,
            quantity: 1,
            parentId: form.location?.id || null,
            entityTypeId: selectedEntityType.value?.id || "",
            tagIds: form.tags,
            manufacturer: "",
            modelNumber: "",
            icon: form.icon,
          });

          if (error) {
            toast.error(t("components.entity.create_modal.batch_failed"));
            break;
          }
          created.push(createdOne);
        }

        if (created.length > 0) {
          toastBatchCreated(created);
          await uploadPhotosToBatch(created.map(e => e.id));
        }

        form.name = "";
        form.quantity = 1;
        form.count = 1;
        form.description = "";
        form.manufacturer = "";
        form.modelNumber = "";
        form.icon = "";
        form.color = "";
        form.photos = [];
        form.tags = [];
        selectedTemplate.value = null;
        templateData.value = null;
        templateUserSelected.value = false;
        showTemplateDetails.value = false;
        aiPrefill.value = false;
        categoryHints.value = [];
        focused.value = false;
        loading.value = false;

        if (close && created[0]) {
          closeDialog(DialogID.CreateEntity);
          navigateTo(`/location/${created[0].id}`);
        } else if (!close) {
          await restoreLastTemplate();
        }
        return;
      }

      let error, data;

      // If the selected entity type is a location, use the location creation endpoint
      if (selectedEntityType.value?.isLocation) {
        const result = await api.items.createLocation({
          name: form.name,
          description: form.description,
          parentId: form.location?.id || null,
          entityTypeId: selectedEntityType.value?.id || "",
          quantity: 1,
          tagIds: form.tags,
          manufacturer: "",
          modelNumber: "",
          icon: form.icon,
        });
        error = result.error;
        data = result.data;
      } else if (templateData.value) {
        // If a template is selected, use the template creation endpoint
        const templateRequest = {
          name: form.name,
          description: form.description,
          parentId: form.location?.id as string,
          tagIds: form.tags,
          quantity: form.quantity,
          entityTypeId: selectedEntityType.value?.id || "",
        };

        const result = await api.templates.createItem(templateData.value.id, templateRequest);
        error = result.error;
        data = result.data;
      } else {
        // Normal item creation without template
        const out: EntityCreate = {
          parentId: form.parentId || (form.location?.id as string),
          name: form.name,
          quantity: form.quantity,
          description: form.description,
          manufacturer: form.manufacturer,
          modelNumber: form.modelNumber,
          tagIds: form.tags,
          entityTypeId: selectedEntityType.value?.id || "",
          icon: form.icon,
        };

        const result = await api.items.create(out);
        error = result.error;
        data = result.data;
      }

      if (error) {
        toast.error(
          t("components.entity.create_modal.toast.create_failed", {
            type: selectedEntityType.value?.name || t("global.entity"),
          })
        );
        return;
      }

      toast.success(
        t("components.entity.create_modal.toast.create_success", {
          type: selectedEntityType.value?.name || t("global.entity"),
        })
      );

      if (form.photos.length > 0) {
        toast.info(
          t("components.entity.create_modal.toast.uploading_photos", {
            count: form.photos.length,
          })
        );
        let uploadError = false;
        for (const photo of form.photos) {
          try {
            const { error: attachError } = await api.items.attachments.add(
              data.id,
              photo.file,
              photo.photoName,
              AttachmentTypes.Photo,
              photo.primary
            );
            if (!attachError) continue;
          } catch {
            // Report network failures through the same per-photo feedback.
          }
          uploadError = true;
          toast.error(
            t("components.entity.create_modal.toast.upload_failed", {
              photoName: photo.photoName,
            })
          );
        }
        if (uploadError) {
          toast.warning(
            t("components.entity.create_modal.toast.some_photos_failed", {
              count: form.photos.length,
            })
          );
        } else {
          toast.success(
            t("components.entity.create_modal.toast.upload_success", {
              count: form.photos.length,
            })
          );
        }
      }

      form.name = "";
      form.quantity = 1;
      form.description = "";
      form.manufacturer = "";
      form.modelNumber = "";
      form.icon = "";
      form.color = "";
      form.photos = [];
      form.tags = [];
      selectedTemplate.value = null;
      templateData.value = null;
      templateUserSelected.value = false;
      showTemplateDetails.value = false;
      aiPrefill.value = false;
      categoryHints.value = [];
      focused.value = false;
      loading.value = false;

      if (close) {
        closeDialog(DialogID.CreateEntity);
        if (selectedEntityType.value?.isLocation) {
          navigateTo(`/location/${data.id}`);
        } else {
          navigateTo(`/item/${data.id}`);
        }
      } else if (!selectedEntityType.value?.isLocation) {
        await restoreLastTemplate();
      }
    } catch {
      toast.error(
        selectedEntityType.value?.isContainer
          ? t("components.entity.create_modal.batch_failed")
          : t("components.entity.create_modal.toast.create_failed", {
              type: selectedEntityType.value?.name || t("global.entity"),
            })
      );
    } finally {
      loading.value = false;
    }
  }

  /**
   * Shared "created N containers" success toast for both batch-creation
   * paths below (template batchCreate + the no-template sequential loop).
   * Its action button fills the label print queue and jumps straight to the
   * generator, so "created a shelf of totes" flows directly into printing
   * labels for them.
   */
  function toastBatchCreated(created: EntityOut[]) {
    const queue = useLabelPrintQueue();
    toast.success(
      t("components.entity.create_modal.batch_created", {
        count: created.length,
      }),
      {
        action: {
          label: t("components.entity.create_modal.print_labels"),
          onClick: () => {
            queue.set(
              created.map(e => ({
                id: e.id,
                kind: "container" as const,
                name: e.name,
                parentPath: form.location?.name ?? "",
                url: `${window.location.origin}/location/${e.id}`,
              }))
            );
            navigateTo("/reports/label-generator");
          },
        },
      }
    );
  }

  /**
   * Uploads the form's pending photos (e.g. a barcode-scanned product image)
   * to every entity created by a batch -- a batch is N copies of the same
   * container/product, so the same photos apply to all of them. Unlike the
   * single-create photo loop in create() below, this reports one aggregate
   * toast for the whole batch instead of one set of toasts per container,
   * since a batch of e.g. 6 totes would otherwise fire 6 "uploading..."/
   * "uploaded" toasts back-to-back.
   */
  async function uploadPhotosToBatch(entityIds: string[]) {
    if (form.photos.length === 0 || entityIds.length === 0) return;

    const totalUploads = form.photos.length * entityIds.length;
    toast.info(
      t("components.entity.create_modal.toast.uploading_photos", {
        count: totalUploads,
      })
    );

    let uploadError = false;
    for (const entityId of entityIds) {
      for (const photo of form.photos) {
        try {
          const { error: attachError } = await api.items.attachments.add(
            entityId,
            photo.file,
            photo.photoName,
            AttachmentTypes.Photo,
            photo.primary
          );
          if (!attachError) continue;
        } catch {
          // Report network failures through the same aggregate feedback.
        }
        uploadError = true;
        toast.error(
          t("components.entity.create_modal.toast.upload_failed", {
            photoName: photo.photoName,
          })
        );
      }
    }

    if (uploadError) {
      toast.warning(
        t("components.entity.create_modal.toast.some_photos_failed", {
          count: totalUploads,
        })
      );
    } else {
      toast.success(
        t("components.entity.create_modal.toast.upload_success", {
          count: totalUploads,
        })
      );
    }
  }

  function openQrScannerPage() {
    openDialog(DialogID.Scanner);
  }

  function openBarcodeDialog() {
    openDialog(DialogID.ProductImport);
  }
</script>
