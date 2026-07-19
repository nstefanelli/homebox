<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import MdiPlus from "~icons/mdi/plus";
  import { Button } from "@/components/ui/button";
  import { useDialog } from "@/components/ui/dialog-provider";
  import { DialogID } from "~/components/ui/dialog-provider/utils";
  import BaseContainer from "@/components/Base/Container.vue";
  import BaseSectionHeader from "@/components/Base/SectionHeader.vue";
  import TemplateCard from "~/components/Template/Card.vue";
  import TemplateCreateModal from "~/components/Template/CreateModal.vue";
  import { useEntityTypeStore } from "~~/stores/entityTypes";
  import { CONTAINER_CATALOG, CONTAINER_TYPE_NAME, catalogFields } from "~~/lib/container-catalog";
  import type { EntityTypeCreate } from "~~/lib/api/types/data-contracts";

  definePageMeta({
    middleware: ["auth"],
  });

  const { t } = useI18n();

  useHead({
    title: computed(() => `HomeBox | ${t("pages.templates.title")}`),
  });

  const api = useUserApi();
  const { openDialog } = useDialog();
  const entityTypeStore = useEntityTypeStore();

  const { data: templates, refresh } = useAsyncData("templates", async () => {
    const { data, error } = await api.templates.getAll();
    if (error) {
      toast.error(t("components.template.toast.load_failed"));
      return [];
    }
    return data;
  });

  // Wrapper functions to match event signatures
  const handleRefresh = () => refresh();
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const handleDuplicated = (_id: string) => refresh();

  const importing = ref(false);

  async function importCatalog() {
    if (importing.value) return;
    importing.value = true;
    try {
      // 1. Ensure the "Tote" container entity type exists (idempotent by name).
      const { data: types, error: typesError } = await api.entityTypes.getAll();
      if (typesError || !Array.isArray(types)) {
        throw new Error("entity type load failed");
      }
      const tote = types.find(et => et.name === CONTAINER_TYPE_NAME);
      if (tote && (!tote.isLocation || !tote.isContainer)) {
        throw new Error("an incompatible entity type named Tote already exists");
      }
      if (!tote) {
        const { error } = await api.entityTypes.create({
          name: CONTAINER_TYPE_NAME,
          isLocation: true,
          isContainer: true,
          icon: "package-variant-closed",
        } as EntityTypeCreate);
        if (error) throw new Error("entity type create failed");
      }
      await entityTypeStore.refresh();

      // 2. Create missing templates (idempotent by name).
      const { data: existing, error: templatesError } = await api.templates.getAll();
      if (templatesError || !Array.isArray(existing)) {
        throw new Error("template load failed");
      }
      const existingNames = new Set(existing.map(tpl => tpl.name));
      let createdCount = 0;
      let failedCount = 0;
      for (const entry of CONTAINER_CATALOG) {
        if (existingNames.has(entry.name)) continue;
        const { error } = await api.templates.create({
          name: entry.name,
          description: `${entry.capacity} container — imported from catalog`,
          notes: "",
          defaultQuantity: 1,
          defaultInsured: false,
          defaultName: entry.name,
          defaultLifetimeWarranty: false,
          includeWarrantyFields: false,
          includePurchaseFields: false,
          includeSoldFields: false,
          fields: catalogFields(entry),
        });
        if (error) {
          failedCount++;
        } else {
          createdCount++;
          existingNames.add(entry.name);
        }
      }
      const skippedCount = CONTAINER_CATALOG.length - createdCount - failedCount;
      if (failedCount > 0) {
        toast.warning(
          t("pages.templates.import_done_with_failures", {
            created: createdCount,
            skipped: skippedCount,
            failed: failedCount,
          })
        );
      } else {
        toast.success(t("pages.templates.import_done", { created: createdCount, skipped: skippedCount }));
      }
      await refresh();
      await refreshNuxtData("templates-selector");
    } catch {
      toast.error(t("pages.templates.import_failed"));
    } finally {
      importing.value = false;
    }
  }
</script>
<template>
  <BaseContainer>
    <div class="mb-4 flex justify-between">
      <BaseSectionHeader>{{ $t("pages.templates.title") }}</BaseSectionHeader>
      <div class="flex gap-2">
        <Button variant="outline" :disabled="importing" @click="importCatalog">
          {{ importing ? $t("pages.templates.importing") : $t("pages.templates.import_catalog") }}
        </Button>
        <Button @click="openDialog(DialogID.CreateTemplate)">
          <MdiPlus class="mr-2" />
          {{ $t("global.create") }}
        </Button>
      </div>
    </div>

    <TemplateCreateModal @created="handleRefresh" />

    <div v-if="templates && templates.length > 0" class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <TemplateCard
        v-for="tpl in templates"
        :key="tpl.id"
        :template="tpl"
        @deleted="handleRefresh"
        @duplicated="handleDuplicated"
      />
    </div>

    <div v-else class="flex flex-col items-center justify-center py-12 text-center">
      <p class="mb-4 text-muted-foreground">{{ $t("pages.templates.no_templates") }}</p>
      <Button @click="openDialog(DialogID.CreateTemplate)">
        <MdiPlus class="mr-2" />
        {{ $t("components.template.create_modal.title") }}
      </Button>
    </div>
  </BaseContainer>
</template>
