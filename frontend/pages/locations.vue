<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { useTreeState } from "~~/components/Location/Tree/tree-state";
  import { useLabelPrintQueue } from "~~/stores/labels";
  import { useLabelSelection } from "~~/stores/labelSelection";
  import MdiCollapseAllOutline from "~icons/mdi/collapse-all-outline";
  import MdiExpandAllOutline from "~icons/mdi/expand-all-outline";
  import MdiPackageVariant from "~icons/mdi/package-variant";

  import { Button, ButtonGroup } from "@/components/ui/button";
  import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
  import type { TreeItem } from "~/lib/api/types/data-contracts";
  import BaseContainer from "@/components/Base/Container.vue";
  import BaseSectionHeader from "@/components/Base/SectionHeader.vue";
  import LocationTreeRoot from "~/components/Location/Tree/Root.vue";
  import BaseCard from "@/components/Base/Card.vue";

  const { t } = useI18n();

  // TODO: eventually move to https://reka-ui.com/docs/components/tree#draggable-sortable-tree

  definePageMeta({
    middleware: ["auth"],
  });

  useHead({
    title: "HomeBox | " + t("menu.locations"),
  });

  const api = useUserApi();

  const selection = useLabelSelection();
  const printQueue = useLabelPrintQueue();
  onUnmounted(() => selection.clear());

  const printingSelected = ref(false);

  async function printSelected() {
    if (printingSelected.value) return;
    printingSelected.value = true;
    let entries = Object.values(selection.selected);
    try {
      // The tree endpoint carries no assetId, so join it in from the
      // entities list (one request, covers locations and containers) before
      // printing. On failure the entries go through without an assetId and
      // the label falls back to its no-ID layout — printing still works.
      const { data, error } = await api.items.getLocations({ filterChildren: false });
      if (!error) {
        const assetIdById = new Map(data.map(l => [l.id, l.assetId]));
        entries = entries.map(e => ({ ...e, assetId: assetIdById.get(e.id) || e.assetId }));
      }
    } catch {
      // Same fallback as the error branch above.
    } finally {
      printingSelected.value = false;
    }
    printQueue.set(entries);
    selection.clear();
    navigateTo("/reports/label-generator");
  }

  const { data: tree } = useAsyncData(async () => {
    const { data, error } = await api.items.getTree({
      withItems: true,
    });

    if (error) {
      return [];
    }

    return data;
  });

  const locationTreeId = "locationTree";
  const showItemsKey = "showItems";

  const treeState = useTreeState(locationTreeId);
  const showItems = ref(true);

  const route = useRouter();

  onMounted(() => {
    // set tree state from query params
    const query = route.currentRoute.value.query;

    if (query && query[locationTreeId]) {
      console.debug("setting tree state from query params");
      const data = JSON.parse(query[locationTreeId] as string);

      for (const key in data) {
        treeState.value[key] = data[key];
      }
    }

    if (query && query[showItemsKey] !== undefined) {
      showItems.value = query[showItemsKey] === "true";
    }
  });

  watch(
    treeState,
    () => {
      // Push the current state to the URL
      route.replace({
        query: {
          [locationTreeId]: JSON.stringify(treeState.value),
          [showItemsKey]: showItems.value.toString(),
        },
      });
    },
    { deep: true }
  );

  watch(showItems, () => {
    route.replace({
      query: {
        [locationTreeId]: JSON.stringify(treeState.value),
        [showItemsKey]: showItems.value.toString(),
      },
    });
  });

  function closeAll() {
    for (const key in treeState.value) {
      treeState.value[key] = false;
    }
  }

  function openItemChildren(items: TreeItem[]) {
    for (const item of items) {
      if (item.children.length > 0) {
        treeState.value[item.id.replace(/-/g, "").substring(0, 8)] = true;
        openItemChildren(item.children);
      }
    }
  }

  function openAll() {
    if (!tree.value) return;

    openItemChildren(tree.value);
  }
</script>

<template>
  <BaseContainer>
    <div class="mb-2 flex justify-between">
      <BaseSectionHeader> {{ $t("menu.locations") }} </BaseSectionHeader>
      <div class="flex items-center gap-2">
        <TooltipProvider :delay-duration="0">
          <ButtonGroup>
            <Tooltip>
              <TooltipTrigger>
                <Button size="icon" variant="outline" data-pos="start" @click="openAll">
                  <MdiExpandAllOutline />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>{{ $t("locations.expand_tree") }}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger>
                <Button size="icon" variant="outline" data-pos="middle" @click="closeAll">
                  <MdiCollapseAllOutline />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>{{ $t("locations.collapse_tree") }}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger>
                <Button
                  size="icon"
                  :variant="showItems ? 'default' : 'outline'"
                  data-pos="end"
                  @click="showItems = !showItems"
                >
                  <MdiPackageVariant />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>{{ showItems ? $t("locations.hide_items") : $t("locations.show_items") }}</p>
              </TooltipContent>
            </Tooltip>
          </ButtonGroup>
        </TooltipProvider>
        <Button variant="outline" @click="selection.selectMode = !selection.selectMode">
          {{ $t("locations.select_labels") }}
        </Button>
        <Button v-if="selection.selectMode && selection.count > 0" @click="printSelected">
          {{ $t("locations.print_selected", { count: selection.count }) }}
        </Button>
      </div>
    </div>
    <BaseCard>
      <div class="p-2">
        <LocationTreeRoot
          v-if="tree && Array.isArray(tree)"
          :locs="tree"
          :tree-id="locationTreeId"
          :show-items="showItems"
        />
      </div>
    </BaseCard>
  </BaseContainer>
</template>
