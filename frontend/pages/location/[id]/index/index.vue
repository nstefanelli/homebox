<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import type { AnyDetail, Details } from "~~/components/global/DetailsSection/types";
  import { filterZeroValues } from "~~/components/global/DetailsSection/types";
  import type { ItemAttachment } from "~~/lib/api/types/data-contracts";
  import { useLabelPrintQueue, type PrintQueueEntry } from "~~/stores/labels";
  import MdiPackageVariant from "~icons/mdi/package-variant";
  import MdiPackageVariantClosed from "~icons/mdi/package-variant-closed";
  import MdiPackageVariantClosedRemove from "~icons/mdi/package-variant-closed-remove";
  import MdiPlus from "~icons/mdi/plus";
  import MdiPencil from "~icons/mdi/pencil";
  import MdiDelete from "~icons/mdi/delete";
  import MdiArrowRight from "~icons/mdi/arrow-right";
  import { useDialog } from "@/components/ui/dialog-provider";
  import { Card } from "@/components/ui/card";
  import {
    Breadcrumb,
    BreadcrumbItem,
    BreadcrumbLink,
    BreadcrumbList,
    BreadcrumbSeparator,
  } from "@/components/ui/breadcrumb";
  import { Button } from "@/components/ui/button";
  import { Badge } from "@/components/ui/badge";
  import { Checkbox } from "@/components/ui/checkbox";
  import { Label } from "@/components/ui/label";
  import { Separator } from "@/components/ui/separator";
  import { DialogID } from "~/components/ui/dialog-provider/utils";
  import BaseCard from "@/components/Base/Card.vue";
  import Currency from "~/components/global/Currency.vue";
  import DateTime from "~/components/global/DateTime.vue";
  import LabelMaker from "~/components/global/LabelMaker.vue";
  import Markdown from "~/components/global/Markdown.vue";
  import DetailsSection from "~/components/global/DetailsSection/DetailsSection.vue";
  import BaseSectionHeader from "@/components/Base/SectionHeader.vue";
  import ItemViewSelectable from "~/components/Item/View/Selectable.vue";
  import ItemAttachmentsList from "~/components/Item/AttachmentsList.vue";
  import ItemImageDialog from "~/components/Item/ImageDialog.vue";
  import LocationCard from "~/components/Location/Card.vue";
  import TagChip from "~/components/Tag/Chip.vue";

  definePageMeta({
    middleware: ["auth"],
  });

  const { t } = useI18n();

  const { openDialog } = useDialog();

  const route = useRoute();
  const api = useUserApi();
  const preferences = useViewPreferences();

  const locationId = computed<string>(() => route.params.id as string);

  const { data: location, refresh: refreshLocation } = useAsyncData(locationId.value, async () => {
    const { data, error } = await api.items.getLocation(locationId.value);
    if (error) {
      toast.error(t("locations.toast.failed_load_location"));
      navigateTo("/home");
      return;
    }

    return data;
  });

  const confirm = useConfirm();

  async function confirmDelete() {
    const { isCanceled } = await confirm.open(t("locations.location_items_delete_confirm"));
    if (isCanceled) {
      return;
    }

    const { error } = await api.items.deleteLocation(locationId.value);
    if (error) {
      toast.error(t("locations.toast.failed_delete_location"));
      return;
    }

    toast.success(t("locations.toast.location_deleted"));
    navigateTo("/locations");
  }

  function openCreateItem() {
    openDialog(DialogID.CreateEntity, {
      params: {
        baseType: "item",
      },
    });
  }

  function goToEdit() {
    navigateTo(`/location/${locationId.value}/edit`);
  }

  function openMove() {
    if (!location.value) return;
    openDialog(DialogID.ItemChangeDetails, {
      params: { items: [location.value], changeLocation: true, currentLocation: location.value },
      onClose: result => {
        if (result) {
          toast.success(t("pages.location.move_success"));
          refreshLocation();
        }
      },
    });
  }

  function openEmpty() {
    if (!location.value || !emptyableChildren.value.length) return;
    openDialog(DialogID.ItemChangeDetails, {
      params: { items: emptyableChildren.value, changeLocation: true, currentLocation: location.value },
      onClose: result => {
        if (result) {
          toast.success(t("pages.location.empty_success"));
          refreshItemList();
          refreshLocation();
          refreshContainersHere();
        }
      },
    });
  }

  // Photos
  type Photo = {
    thumbnailSrc?: string;
    originalSrc: string;
    attachmentId: string;
    originalType?: string;
  };

  const photos = computed<Photo[]>(() => {
    if (!location.value?.attachments) {
      return [];
    }
    return location.value.attachments.reduce((acc, cur) => {
      if (cur.type === "photo") {
        const photo: Photo = {
          originalSrc: api.authURL(`/entities/${location.value!.id}/attachments/${cur.id}`),
          originalType: cur.mimeType,
          attachmentId: cur.id,
        };
        if (cur.thumbnail) {
          photo.thumbnailSrc = api.authURL(`/entities/${location.value!.id}/attachments/${cur.thumbnail.id}`);
        } else {
          photo.thumbnailSrc = photo.originalSrc;
        }
        acc.push(photo);
      }
      return acc;
    }, [] as Photo[]);
  });

  function openImageDialog(img: Photo, entityId: string) {
    openDialog(DialogID.ItemImage, {
      params: {
        type: "preloaded",
        originalSrc: img.originalSrc,
        originalType: img.originalType,
        thumbnailSrc: img.thumbnailSrc,
        attachmentId: img.attachmentId,
        itemId: entityId,
      },
      onClose: result => {
        if (result?.action === "delete") {
          location.value!.attachments = location.value!.attachments.filter(a => a.id !== result.id);
        }
      },
    });
  }

  // Attachments (non-photo)
  const nonPhotoAttachments = computed(() => {
    if (!location.value?.attachments) {
      return { attachments: [], warranty: [], manuals: [], receipts: [] };
    }
    return location.value.attachments.reduce(
      (acc, attachment) => {
        if (attachment.type === "photo") return acc;
        if (attachment.type === "warranty") acc.warranty.push(attachment);
        else if (attachment.type === "manual") acc.manuals.push(attachment);
        else if (attachment.type === "receipt") acc.receipts.push(attachment);
        else acc.attachments.push(attachment);
        return acc;
      },
      {
        attachments: [] as ItemAttachment[],
        warranty: [] as ItemAttachment[],
        manuals: [] as ItemAttachment[],
        receipts: [] as ItemAttachment[],
      }
    );
  });

  const hasNonPhotoAttachments = computed(() => {
    const a = nonPhotoAttachments.value;
    return a.attachments.length > 0 || a.warranty.length > 0 || a.manuals.length > 0 || a.receipts.length > 0;
  });

  // Details
  const locationDetails = computed<Details>(() => {
    if (!location.value) {
      return [];
    }

    const ret: Details = [
      {
        name: "items.notes",
        type: "markdown",
        text: location.value.notes,
      },
      ...(location.value.fields || []).map(field => {
        return {
          name: field.name,
          text: field.textValue,
        } as AnyDetail;
      }),
    ];

    if (!preferences.value.showEmpty) {
      return filterZeroValues(ret);
    }

    return ret;
  });

  const { data: items, refresh: refreshItemList } = useAsyncData(
    () => locationId.value + "_item_list",
    async () => {
      if (!locationId.value) {
        return [];
      }

      const resp = await api.items.getAll({
        parentIds: [locationId.value],
      });

      if (resp.error) {
        toast.error(t("items.toast.failed_load_items"));
        return [];
      }

      return resp.data.items;
    },
    {
      watch: [locationId],
    }
  );

  const { data: containersHere, refresh: refreshContainersHere } = useAsyncData(
    () => locationId.value + "_containers_here",
    async () => {
      if (!locationId.value) {
        return [];
      }

      const { data } = await api.items.getContainers({ parentIds: [locationId.value], filterChildren: false });
      return data;
    },
    {
      watch: [locationId],
    }
  );

  // Wrapped in a computed (rather than referencing the useAsyncData ref directly in the template) to
  // avoid vue-tsc's ref-unwrap quirk with bare async-data refs in v-if/v-for guards.
  const containersHereList = computed(() => containersHere.value ?? []);

  // Child locations that aren't already shown in the "Containers here" section, to avoid double-rendering.
  const childLocations = computed(() => {
    if (!location.value?.children) {
      return [];
    }
    const containerIds = new Set(containersHereList.value.map(c => c.id));
    return location.value.children.filter(child => !containerIds.has(child.id));
  });

  // All direct children eligible to be moved by "Empty container": child items, plus the complete
  // deduped set of direct location-type children. `containersHereList` and `childLocations` were split
  // above purely to avoid double-rendering the "Containers here" vs "Child locations" sections — recombining
  // them here (rather than reusing `location.children`) recovers the full direct-children set with no
  // double-counting, since `childLocations` already excludes anything present in `containersHereList`.
  const emptyableChildren = computed(() => [
    ...(items.value ?? []),
    ...containersHereList.value,
    ...childLocations.value,
  ]);

  // Wrapped in a computed (rather than referencing `location.entityType` directly in the template) to
  // avoid vue-tsc's ref-unwrap quirk with bare async-data refs — same reasoning as containersHereList above.
  const canEmptyContainer = computed(
    () => !!location.value?.entityType?.isContainer && emptyableChildren.value.length > 0
  );

  const printQueue = useLabelPrintQueue();
  const printIncludeItems = ref(false);

  async function printContainerLabels() {
    const entries: PrintQueueEntry[] = [];

    const { data: containers } = await api.items.getContainers({
      parentIds: [locationId.value],
      filterChildren: false,
    });
    entries.push(
      ...containers.map(c => ({
        id: c.id,
        kind: "container" as const,
        name: c.name,
        parentPath: location.value?.name ?? "",
        url: `${window.location.origin}/location/${c.id}`,
      }))
    );

    if (printIncludeItems.value) {
      entries.push(
        ...(items.value ?? []).map(i => ({
          id: i.id,
          kind: "item" as const,
          name: i.name,
          parentPath: location.value?.name ?? "",
          assetId: i.assetId,
          url: `${window.location.origin}/a/${i.assetId}`,
        }))
      );
    }

    printQueue.set(entries);
    navigateTo("/reports/label-generator");
  }
</script>

<template>
  <div>
    <ItemImageDialog />

    <div v-if="location">
      <!-- set page title -->
      <Title>{{ location.name }}</Title>

      <!-- Photo gallery -->
      <section v-if="photos.length > 0" class="mb-4">
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
          <button
            v-for="(photo, i) in photos"
            :key="i"
            class="group relative aspect-1 h-32 overflow-hidden rounded-lg border bg-muted"
            @click="openImageDialog(photo, location.id)"
          >
            <img
              :src="photo.thumbnailSrc || photo.originalSrc"
              :alt="location.name"
              class="size-full object-cover transition-transform duration-200 group-hover:scale-105"
            />
          </button>
        </div>
      </section>

      <Card class="p-3">
        <header :class="{ 'mb-2': location?.description }">
          <div class="flex flex-wrap items-end gap-2">
            <div
              class="mb-auto flex size-12 items-center justify-center rounded-full bg-secondary text-secondary-foreground"
            >
              <MdiPackageVariant class="size-7" />
            </div>
            <div>
              <Breadcrumb v-if="location?.parent">
                <BreadcrumbList>
                  <BreadcrumbItem>
                    <BreadcrumbLink as-child class="text-foreground/70 hover:underline">
                      <NuxtLink :to="`/location/${location.parent.id}`">
                        {{ location.parent.name }}
                      </NuxtLink>
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem> {{ location.name }} </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <h1 class="flex items-center gap-3 pb-1 text-2xl">
                {{ location ? location.name : "" }}

                <Badge v-if="location && location.totalPrice" variant="secondary">
                  <Currency :amount="location.totalPrice" />
                </Badge>
              </h1>
              <div class="flex flex-wrap gap-1 text-xs">
                <div>
                  {{ $t("global.created") }}
                  <DateTime :date="location?.createdAt" />
                </div>
              </div>
              <div v-if="location.tags && location.tags.length > 0" class="mt-2 flex flex-wrap gap-1">
                <TagChip v-for="tag in location.tags" :key="tag.id" :tag="tag" size="sm" />
              </div>
            </div>
            <div class="ml-auto mt-2 flex flex-wrap items-center justify-between gap-2">
              <LabelMaker :id="location.id" type="location" />
              <Button variant="outline" @click="printContainerLabels">
                {{ $t("components.location.print_containers") }}
              </Button>
              <div class="flex items-center gap-1">
                <Checkbox id="printIncludeItems" v-model="printIncludeItems" />
                <Label for="printIncludeItems" class="cursor-pointer text-sm">
                  {{ $t("components.location.print_include_items") }}
                </Label>
              </div>
              <Button class="w-9 md:w-auto" @click="openCreateItem">
                <MdiPlus name="mdi-plus" />
                <span class="hidden md:inline">
                  {{ $t("components.location.create_item") }}
                </span>
              </Button>
              <Button class="w-9 md:w-auto" @click="goToEdit">
                <MdiPencil name="mdi-pencil" />
                <span class="hidden md:inline">
                  {{ $t("global.edit") }}
                </span>
              </Button>
              <Button class="w-9 md:w-auto" variant="outline" @click="openMove">
                <MdiArrowRight name="mdi-arrow-right" />
                <span class="hidden md:inline">
                  {{ $t("pages.location.move") }}
                </span>
              </Button>
              <Button v-if="canEmptyContainer" class="w-9 md:w-auto" variant="outline" @click="openEmpty">
                <MdiPackageVariantClosedRemove name="mdi-package-variant-closed-remove" />
                <span class="hidden md:inline">
                  {{ $t("pages.location.empty_container") }}
                </span>
              </Button>
              <Button variant="destructive" class="w-9 md:w-auto" @click="confirmDelete()">
                <MdiDelete name="mdi-delete" />
                <span class="hidden md:inline">
                  {{ $t("global.delete") }}
                </span>
              </Button>
            </div>
          </div>
        </header>
        <Separator v-if="location && location.description" />
        <Markdown v-if="location && location.description" class="mt-3 text-base" :source="location.description" />
      </Card>

      <!-- Details (notes, custom fields) -->
      <BaseCard v-if="locationDetails.length > 0" class="mt-4">
        <template #title> {{ $t("global.details") }} </template>
        <DetailsSection :details="locationDetails" />
      </BaseCard>

      <!-- Attachments (non-photo) -->
      <BaseCard v-if="hasNonPhotoAttachments" class="mt-4">
        <template #title> {{ $t("items.attachments") }} </template>
        <div class="border-t px-4 py-2">
          <ItemAttachmentsList
            v-if="nonPhotoAttachments.attachments.length > 0"
            :attachments="nonPhotoAttachments.attachments"
            :item-id="location.id"
          />
          <ItemAttachmentsList
            v-if="nonPhotoAttachments.warranty.length > 0"
            :attachments="nonPhotoAttachments.warranty"
            :item-id="location.id"
          />
          <ItemAttachmentsList
            v-if="nonPhotoAttachments.manuals.length > 0"
            :attachments="nonPhotoAttachments.manuals"
            :item-id="location.id"
          />
          <ItemAttachmentsList
            v-if="nonPhotoAttachments.receipts.length > 0"
            :attachments="nonPhotoAttachments.receipts"
            :item-id="location.id"
          />
        </div>
      </BaseCard>

      <!-- Items in this location -->
      <section v-if="location && items">
        <ItemViewSelectable :items="items" @refresh="refreshItemList" />
      </section>

      <!-- Containers here -->
      <section v-if="containersHereList.length > 0" class="mt-6 space-y-2">
        <BaseSectionHeader class="mb-5">
          {{ $t("pages.location.containers_here", { count: containersHereList.length }) }}
        </BaseSectionHeader>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <NuxtLink
            v-for="c in containersHereList"
            :key="c.id"
            :to="`/location/${c.id}`"
            class="flex items-center justify-between rounded-md border p-3 hover:bg-accent"
          >
            <span class="flex items-center gap-2">
              <MdiPackageVariantClosed class="size-4" />
              {{ c.name }}
            </span>
            <Badge v-if="c.itemCount != null" variant="secondary">
              {{ $t("pages.location.container_item_count", { count: c.itemCount }) }}
            </Badge>
          </NuxtLink>
        </div>
      </section>

      <!-- Child locations -->
      <section v-if="location && childLocations.length > 0" class="mt-6">
        <BaseSectionHeader class="mb-5"> {{ $t("locations.child_locations") }} </BaseSectionHeader>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
          <LocationCard v-for="child in childLocations" :key="child.id" :location="child" />
        </div>
      </section>
    </div>
  </div>
</template>
