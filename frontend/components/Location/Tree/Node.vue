<script setup lang="ts">
  import { useTreeState } from "./tree-state";
  import type { TreeItem } from "~~/lib/api/types/data-contracts";
  import { useLabelSelection } from "~~/stores/labelSelection";
  import { Checkbox } from "@/components/ui/checkbox";
  import MdiChevronRight from "~icons/mdi/chevron-right";
  import MdiPackageVariant from "~icons/mdi/package-variant";
  import { resolveEntityIcon } from "~~/lib/icons";
  import LocationTreeNode from "./Node.vue";

  type Props = {
    treeId: string;
    item: TreeItem;
    showItems?: boolean;
  };
  const props = withDefaults(defineProps<Props>(), {
    showItems: true,
  });

  const link = computed(() => {
    return props.item.type === "location" ? `/location/${props.item.id}` : `/item/${props.item.id}`;
  });

  const state = useTreeState(props.treeId);

  const selection = useLabelSelection();

  function toggleSelect() {
    selection.toggle({
      id: props.item.id,
      kind: "location",
      name: props.item.name,
      url: `${window.location.origin}/location/${props.item.id}`,
    });
  }

  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

  const filteredChildren = computed(() => {
    const children = props.item.children ?? [];

    if (props.showItems) {
      return children;
    }

    return children.filter(child => child.type === "location");
  });

  const sortedChildren = computed(() => {
    return [...filteredChildren.value].sort((a, b) => collator.compare(a.name, b.name));
  });

  const hasChildren = computed(() => filteredChildren.value.length > 0);

  const openRef = computed({
    get() {
      return state.value[nodeHash.value] ?? false;
    },
    set(value: boolean) {
      state.value[nodeHash.value] = value;
    },
  });

  const nodeHash = computed(() => {
    // converts a UUID to a short hash
    return props.item.id.replace(/-/g, "").substring(0, 8);
  });

  const nodeIcon = computed(() => {
    if (props.item.type !== "location") {
      return MdiPackageVariant; // item nodes keep their existing icon
    }
    return resolveEntityIcon({
      icon: props.item.icon,
      typeIcon: props.item.typeIcon,
      isContainer: props.item.isContainer,
      isLocation: true,
    });
  });
</script>

<template>
  <div>
    <div
      class="flex items-center gap-1 rounded p-1"
      :class="{
        'cursor-pointer hover:bg-accent hover:text-accent-foreground': hasChildren,
      }"
      @click="openRef = !openRef"
    >
      <div
        class="mr-1 flex items-center justify-center rounded p-0.5"
        :class="{
          'hover:bg-accent hover:text-accent-foreground': hasChildren,
        }"
      >
        <div v-if="!hasChildren" class="size-6" />
        <div v-else class="group/node relative size-6" :data-swap="openRef">
          <div
            class="absolute inset-0 flex items-center justify-center transition-transform duration-300 group-data-[swap=true]/node:rotate-90"
          >
            <MdiChevronRight class="size-6" />
          </div>
        </div>
      </div>
      <Checkbox
        v-if="selection.selectMode && item.type === 'location'"
        :model-value="!!selection.selected[item.id]"
        class="mr-2"
        @update:model-value="toggleSelect"
        @click.stop
      />
      <component :is="nodeIcon" class="size-4" />
      <NuxtLink class="text-lg hover:underline" :to="link" @click.stop>{{ item.name }} </NuxtLink>
    </div>
    <div v-if="openRef" class="ml-4">
      <LocationTreeNode
        v-for="child in sortedChildren"
        :key="child.id"
        :item="child"
        :tree-id="treeId"
        :show-items="showItems"
      />
    </div>
  </div>
</template>
