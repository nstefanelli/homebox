<template>
  <div class="space-y-2">
    <Label>{{ $t("components.template.photo.title") }}</Label>
    <div class="flex items-center gap-4">
      <img
        v-if="hasPhoto"
        :src="photoSrc"
        class="size-24 rounded-md border object-cover"
        :alt="$t('components.template.photo.title')"
      />
      <div class="flex flex-col gap-2">
        <Input type="file" accept="image/*" @change="onFileChange" />
        <Button v-if="hasPhoto" variant="destructive" size="sm" type="button" @click="removePhoto">
          {{ $t("components.template.photo.remove") }}
        </Button>
      </div>
    </div>
    <p class="text-xs text-muted-foreground">{{ $t("components.template.photo.hint") }}</p>
  </div>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { computed, ref } from "vue";
  import { toast } from "@/components/ui/sonner";
  import { Button } from "@/components/ui/button";
  import { Input } from "@/components/ui/input";
  import { Label } from "@/components/ui/label";

  const props = defineProps<{ templateId: string; photoPath?: string }>();
  const emit = defineEmits<{ updated: [] }>();

  const api = useUserApi();
  const { t } = useI18n();

  const cacheBust = ref(0);
  const localHasPhoto = ref<boolean | null>(null);
  const hasPhoto = computed(() => localHasPhoto.value ?? !!props.photoPath);
  const photoSrc = computed(() => {
    const base = api.templates.photoUrl(props.templateId);
    const separator = base.includes("?") ? "&" : "?";
    return `${base}${separator}v=${cacheBust.value}`;
  });

  async function onFileChange(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    const { error } = await api.templates.uploadPhoto(props.templateId, file);
    if (error) {
      toast.error(t("components.template.photo.upload_failed"));
      return;
    }
    localHasPhoto.value = true;
    cacheBust.value++;
    input.value = "";
    emit("updated");
  }

  async function removePhoto() {
    const { error } = await api.templates.deletePhoto(props.templateId);
    if (error) {
      toast.error(t("components.template.photo.delete_failed"));
      return;
    }
    localHasPhoto.value = false;
    emit("updated");
  }
</script>
