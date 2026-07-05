<script setup lang="ts">
  import { DialogRoot, type DialogRootEmits, type DialogRootProps, useForwardPropsEmits } from "reka-ui";
  import { useDialog, type DialogID } from "@/components/ui/dialog-provider/utils";

  // beforeClose is optional and backward-compatible: when a caller doesn't
  // pass it, onOpenChange behaves exactly as before this prop existed. When
  // provided, it's consulted for every close request that reaches this
  // component -- which covers ALL of reka-ui's built-in dismissal paths
  // (DialogClose "x" button, Escape, outside-click) since they all funnel
  // through DialogRoot's onOpenChange(false) -> `update:open` -> here.
  // Returning false keeps the dialog open.
  const props = defineProps<DialogRootProps & { dialogId: DialogID; beforeClose?: () => boolean | Promise<boolean> }>();
  const emits = defineEmits<DialogRootEmits>();

  const { closeDialog, activeDialog } = useDialog();

  const isOpen = computed(() => (activeDialog.value && activeDialog.value === props.dialogId) ?? false);

  const delegatedProps = computed(() => {
    const { dialogId, beforeClose, ...delegated } = props;
    return delegated;
  });

  const onOpenChange = async (open: boolean) => {
    if (open) return;
    if (props.beforeClose && !(await props.beforeClose())) {
      return;
    }
    closeDialog(props.dialogId);
  };

  const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <DialogRoot v-bind="forwarded" :open="isOpen" @update:open="onOpenChange">
    <slot />
  </DialogRoot>
</template>
