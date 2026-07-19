<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
  import { Button } from "@/components/ui/button";
  import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
  import MdiDelete from "~icons/mdi/delete";
  import { toast } from "@/components/ui/sonner";
  import type { UserSummary } from "~~/lib/api/types/data-contracts";
  import { useIntegrationsStore } from "~~/stores/integrations";

  definePageMeta({
    middleware: ["auth"],
  });

  const { t } = useI18n();

  useHead({ title: `HomeBox | ${t("collection.tabs.members")}` });

  const api = useUserApi();
  const auth = useAuthContext();
  const confirm = useConfirm();
  const integrationsStore = useIntegrationsStore();

  const loading = ref(true);
  const members = ref<UserSummary[]>([]);
  const error = ref<string | null>(null);
  const removing = ref<Record<string, boolean>>({});

  const currentUserId = computed(() => auth.user?.id ?? "");

  const loadMembers = async () => {
    loading.value = true;
    error.value = null;

    try {
      const res = await api.group.getMembers();
      if (res.error) {
        const msg = t("errors.api_failure") + String(res.error);
        error.value = msg;
        members.value = [];
        toast.error(msg);
      } else {
        members.value = res.data ?? [];
      }
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      error.value = msg;
      members.value = [];
      toast.error(msg);
    } finally {
      loading.value = false;
    }
  };

  const handleRemove = async (user: UserSummary) => {
    if (!integrationsStore.isOwner || !currentUserId.value || !user?.id || user.id === currentUserId.value) return;

    const result = await confirm.open(t("collection.members.remove_confirm"));
    if (result.isCanceled) {
      return;
    }

    removing.value = { ...removing.value, [user.id]: true };

    try {
      const res = await api.group.removeMember(user.id);
      if (res.error) {
        const msg = t("errors.api_failure") + String(res.error);
        toast.error(msg);
      } else {
        members.value = members.value.filter(m => m.id !== user.id);
        toast.success(t("collection.members.removed"));
      }
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      toast.error(msg);
    } finally {
      removing.value = { ...removing.value, [user.id]: false };
    }
  };

  onMounted(async () => {
    void loadMembers();
    try {
      await integrationsStore.ensureFetched();
    } catch {
      toast.error(t("collection.permissions_load_failed"));
    }
  });
</script>

<template>
  <div class="space-y-4">
    <div v-if="loading" class="rounded-md border bg-card p-4 text-sm text-muted-foreground">
      {{ $t("global.loading") }}
    </div>

    <div v-else>
      <div v-if="!members.length" class="rounded-md border bg-card p-4 text-sm text-muted-foreground">
        {{ $t("collection.members.empty") }}
      </div>

      <div v-else class="scroll-bg overflow-x-auto rounded-md border bg-card">
        <Table class="min-w-[480px]">
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t("collection.members.name") }}</TableHead>
              <TableHead>{{ $t("collection.members.email") }}</TableHead>
              <TableHead v-if="integrationsStore.isOwner" class="w-32 text-right">
                <span class="sr-only">{{ $t("collection.actions") }}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="user in members" :key="user.id">
              <TableCell>{{ user.name }}</TableCell>
              <TableCell>{{ user.email }}</TableCell>
              <TableCell v-if="integrationsStore.isOwner">
                <div class="ml-auto">
                  <TooltipProvider v-if="currentUserId && user.id !== currentUserId" :delay-duration="0">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <Button
                          variant="destructive"
                          size="icon"
                          :aria-label="$t('collection.members.remove')"
                          :disabled="removing[user.id]"
                          @click="handleRemove(user)"
                        >
                          <MdiDelete class="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>
                        {{ $t("collection.members.remove") }}
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </div>
  </div>
</template>
