<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import { Button } from "@/components/ui/button";
  import { Label } from "@/components/ui/label";
  import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
  import MdiLoading from "~icons/mdi/loading";
  import MdiRobot from "~icons/mdi/robot";
  import MdiBarcode from "~icons/mdi/barcode";
  import MdiClose from "~icons/mdi/close";
  import MdiCheck from "~icons/mdi/check";
  import MdiAlertCircleOutline from "~icons/mdi/alert-circle-outline";
  import FormTextField from "~/components/Form/TextField.vue";
  import FormPassword from "~/components/Form/Password.vue";
  import { useIntegrationsStore } from "~~/stores/integrations";
  import type {
    CurrenciesCurrency,
    Group,
    GroupIntegrationsOut,
    TestConnectionResponse,
    TypesGroupIntegrations,
  } from "~~/lib/api/types/data-contracts";
  import { fmtCurrencyAsync } from "~/composables/utils";

  definePageMeta({
    middleware: ["auth"],
  });

  const { t } = useI18n();

  useHead({ title: `HomeBox | ${t("collection.tabs.settings")}` });

  const api = useUserApi();
  const { selectedCollection, load: reloadCollections } = useCollections();
  const integrationsStore = useIntegrationsStore();

  const loading = ref(true);
  const saving = ref(false);
  const error = ref<string | null>(null);

  const group = ref<Group | null>(null);
  const currencies = ref<CurrenciesCurrency[]>([]);
  const name = ref("");
  const currencyCode = ref("USD");
  const currencyExample = ref("$1,000.00");

  const loadSettings = async () => {
    if (!selectedCollection.value) {
      loading.value = false;
      return;
    }

    loading.value = true;
    error.value = null;

    try {
      if (!currencies.value.length) {
        const respCurrencies = await api.group.currencies();
        if (respCurrencies.error) {
          toast.error(t("profile.toast.failed_get_currencies"));
        } else if (respCurrencies.data) {
          currencies.value = respCurrencies.data;
        }
      }

      const res = await api.group.get(selectedCollection.value.id);
      if (res.error || !res.data) {
        const msg = t("errors.api_failure") + String(res.error ?? "");
        error.value = msg;
        toast.error(msg);
        return;
      }

      group.value = res.data;
      name.value = res.data.name;
      currencyCode.value = res.data.currency;
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      error.value = msg;
      toast.error(msg);
    } finally {
      loading.value = false;
    }
  };

  watch(
    () => selectedCollection.value?.id,
    () => {
      void loadSettings();
    },
    { immediate: true }
  );

  watch(
    currencyCode,
    async () => {
      if (!currencyCode.value) return;
      try {
        currencyExample.value = await fmtCurrencyAsync(1000, currencyCode.value, getLocaleCode());
      } catch {
        currencyExample.value = `${currencyCode.value} 1000`;
      }
    },
    { immediate: true }
  );

  const save = async () => {
    if (!selectedCollection.value) return;

    saving.value = true;
    error.value = null;

    try {
      const res = await api.group.update(
        {
          name: name.value,
          currency: currencyCode.value,
        },
        selectedCollection.value.id
      );

      if (res.error || !res.data) {
        const msg = t("profile.toast.failed_update_group");
        error.value = msg;
        toast.error(msg);
        return;
      }

      group.value = res.data;
      setCurrency(res.data.currency);
      toast.success(t("profile.toast.group_updated"));

      await reloadCollections();
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      error.value = msg;
      toast.error(msg);
    } finally {
      saving.value = false;
    }
  };

  // ---------------------------------------------------------------------
  // Integrations card
  // ---------------------------------------------------------------------

  // Mirrors config.RedactedValue on the backend (backend/internal/sys/config/conf.go).
  // A secret field GET-returns this literal (never the plaintext) whenever a
  // value is configured, either on the group itself or via env fallback.
  const REDACTED_SENTINEL = "[REDACTED]";

  // The Select component treats an empty-string item value as "no selection",
  // so the "inherit env default" option is represented internally by this
  // sentinel and mapped back to "" (the wire value) at the edges.
  const PROVIDER_INHERIT_OPTION = "inherit";

  type SecretFieldState = {
    sentinelPresent: boolean;
    value: string;
    cleared: boolean;
  };

  function secretFieldFromRaw(raw: string): SecretFieldState {
    return {
      sentinelPresent: raw === REDACTED_SENTINEL,
      value: "",
      cleared: false,
    };
  }

  // On save: typed text always wins; otherwise an explicit clear sends "";
  // otherwise an untouched field that was showing the saved-sentinel keeps
  // it by echoing the sentinel back; otherwise there's nothing to keep.
  function secretFieldPayload(state: SecretFieldState): string {
    if (state.value !== "") return state.value;
    if (state.cleared) return "";
    if (state.sentinelPresent) return REDACTED_SENTINEL;
    return "";
  }

  const integrationsLoading = ref(true);
  const savingIntegrations = ref(false);
  const testingAI = ref(false);
  const testingBarcode = ref(false);

  const aiProvider = ref("");
  const aiBaseUrl = ref("");
  const aiModel = ref("");
  const aiApiKey = ref<SecretFieldState>(secretFieldFromRaw(""));

  const barcodeToken = ref<SecretFieldState>(secretFieldFromRaw(""));
  const openFoodFactsContact = ref("");

  const envAiProvider = ref("");
  const envAiBaseUrl = ref("");
  const envAiModel = ref("");

  const testAIResult = ref<TestConnectionResponse | null>(null);
  const testBarcodeResult = ref<TestConnectionResponse | null>(null);

  const providerOptions = computed(() => [
    { value: PROVIDER_INHERIT_OPTION, label: t("profile.integrations.provider_inherit") },
    { value: "disabled", label: t("profile.integrations.provider_disabled") },
    { value: "openai_compatible", label: t("profile.integrations.provider_openai_compatible") },
    { value: "anthropic", label: t("profile.integrations.provider_anthropic") },
  ]);

  const providerSelectValue = computed({
    get: () => (aiProvider.value === "" ? PROVIDER_INHERIT_OPTION : aiProvider.value),
    set: (val: string) => {
      aiProvider.value = val === PROVIDER_INHERIT_OPTION ? "" : val;
    },
  });

  const aiApiKeyValue = computed({
    get: () => aiApiKey.value.value,
    set: (val: string) => {
      aiApiKey.value = { ...aiApiKey.value, value: val };
    },
  });

  const barcodeTokenValue = computed({
    get: () => barcodeToken.value.value,
    set: (val: string) => {
      barcodeToken.value = { ...barcodeToken.value, value: val };
    },
  });

  const aiApiKeyPlaceholder = computed(() =>
    aiApiKey.value.sentinelPresent && !aiApiKey.value.cleared ? t("profile.integrations.secret_saved_placeholder") : ""
  );

  const barcodeTokenPlaceholder = computed(() =>
    barcodeToken.value.sentinelPresent && !barcodeToken.value.cleared
      ? t("profile.integrations.secret_saved_placeholder")
      : ""
  );

  const aiApiKeyClearable = computed(
    () => (aiApiKey.value.sentinelPresent && !aiApiKey.value.cleared) || aiApiKey.value.value !== ""
  );

  const barcodeTokenClearable = computed(
    () => (barcodeToken.value.sentinelPresent && !barcodeToken.value.cleared) || barcodeToken.value.value !== ""
  );

  function clearAiApiKey() {
    aiApiKey.value = { sentinelPresent: aiApiKey.value.sentinelPresent, value: "", cleared: true };
  }

  function clearBarcodeToken() {
    barcodeToken.value = { sentinelPresent: barcodeToken.value.sentinelPresent, value: "", cleared: true };
  }

  const envHint = computed(() => {
    if (!envAiProvider.value) return "";
    if (envAiModel.value) {
      return t("profile.integrations.env_hint_with_model", {
        provider: envAiProvider.value,
        baseUrl: envAiBaseUrl.value,
        model: envAiModel.value,
      });
    }
    return t("profile.integrations.env_hint", {
      provider: envAiProvider.value,
      baseUrl: envAiBaseUrl.value,
    });
  });

  function applyIntegrationsData(data: GroupIntegrationsOut) {
    aiProvider.value = data.aiProvider;
    aiBaseUrl.value = data.aiBaseUrl;
    aiModel.value = data.aiModel;
    aiApiKey.value = secretFieldFromRaw(data.aiApiKey);

    barcodeToken.value = secretFieldFromRaw(data.barcodeTokenBarcodespider);
    openFoodFactsContact.value = data.openFoodFactsContact;

    envAiProvider.value = data.envAiProvider;
    envAiBaseUrl.value = data.envAiBaseUrl;
    envAiModel.value = data.envAiModel;

    testAIResult.value = null;
    testBarcodeResult.value = null;
  }

  const loadIntegrations = async () => {
    if (!selectedCollection.value) {
      integrationsLoading.value = false;
      return;
    }

    integrationsLoading.value = true;

    try {
      await integrationsStore.refresh();
      if (integrationsStore.data) {
        applyIntegrationsData(integrationsStore.data);
      }
    } catch (e) {
      toast.error(t("profile.integrations.toast.failed_load"));
    } finally {
      integrationsLoading.value = false;
    }
  };

  watch(
    () => selectedCollection.value?.id,
    () => {
      void loadIntegrations();
    },
    { immediate: true }
  );

  const saveIntegrations = async () => {
    if (!selectedCollection.value || !integrationsStore.isOwner) return;

    savingIntegrations.value = true;

    try {
      const body: TypesGroupIntegrations = {
        aiProvider: aiProvider.value,
        aiBaseUrl: aiBaseUrl.value,
        aiModel: aiModel.value,
        aiApiKey: secretFieldPayload(aiApiKey.value),
        barcodeTokenBarcodespider: secretFieldPayload(barcodeToken.value),
        openFoodFactsContact: openFoodFactsContact.value,
      };

      const res = await api.group.updateIntegrations(body);
      if (res.error || !res.data) {
        toast.error(t("profile.integrations.toast.failed_save"));
        return;
      }

      await integrationsStore.refresh();
      if (integrationsStore.data) {
        applyIntegrationsData(integrationsStore.data);
      }

      toast.success(t("profile.integrations.toast.saved"));
    } catch (e) {
      toast.error(t("profile.integrations.toast.failed_save"));
    } finally {
      savingIntegrations.value = false;
    }
  };

  const testAI = async () => {
    testingAI.value = true;
    testAIResult.value = null;

    try {
      const res = await api.group.testAI();
      if (res.error || !res.data) {
        testAIResult.value = { ok: false, detail: t("profile.integrations.toast.test_failed") };
        return;
      }
      testAIResult.value = res.data;
    } catch (e) {
      testAIResult.value = { ok: false, detail: t("profile.integrations.toast.test_failed") };
    } finally {
      testingAI.value = false;
    }
  };

  const testBarcode = async () => {
    testingBarcode.value = true;
    testBarcodeResult.value = null;

    try {
      const res = await api.group.testBarcode();
      if (res.error || !res.data) {
        testBarcodeResult.value = { ok: false, detail: t("profile.integrations.toast.test_failed") };
        return;
      }
      testBarcodeResult.value = res.data;
    } catch (e) {
      testBarcodeResult.value = { ok: false, detail: t("profile.integrations.toast.test_failed") };
    } finally {
      testingBarcode.value = false;
    }
  };
</script>

<template>
  <div class="space-y-4">
    <div v-if="loading" class="rounded-md border bg-card p-4 text-sm text-muted-foreground">
      {{ $t("global.loading") }}
    </div>

    <div v-else>
      <div v-if="!selectedCollection" class="rounded-md border bg-card p-4 text-sm text-muted-foreground">
        {{ $t("components.collection.selector.select_collection") }}
      </div>

      <div v-else class="space-y-4">
        <div class="space-y-4 rounded-md border bg-card p-4">
          <FormTextField v-model="name" :label="$t('global.name')" />

          <div>
            <Label for="currency"> {{ $t("profile.currency_format") }} </Label>
            <Select
              id="currency"
              :model-value="currencyCode"
              @update:model-value="val => (currencyCode = String(val || ''))"
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="c in currencies" :key="c.code" :value="c.code">
                  {{ c.name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <p class="m-2 text-sm">{{ $t("profile.example") }}: {{ currencyExample }}</p>
          </div>

          <div class="mt-4">
            <Button variant="secondary" size="sm" :disabled="saving" @click="save">
              <MdiLoading v-if="saving" class="mr-2 inline-block animate-spin" />
              <span>{{ $t("profile.update_group") }}</span>
            </Button>
          </div>
        </div>

        <div class="space-y-4 rounded-md border bg-card p-4">
          <header class="flex items-center gap-2">
            <div>
              <h2 class="text-lg font-semibold">{{ $t("profile.integrations.title") }}</h2>
              <p class="text-sm text-muted-foreground">{{ $t("profile.integrations.subtitle") }}</p>
            </div>
          </header>

          <div v-if="integrationsLoading" class="text-sm text-muted-foreground">
            {{ $t("global.loading") }}
          </div>

          <template v-else>
            <p v-if="!integrationsStore.isOwner" class="rounded-md border bg-muted p-2 text-sm text-muted-foreground">
              {{ $t("profile.integrations.owner_only_notice") }}
            </p>

            <section class="space-y-4">
              <h3 class="flex items-center gap-2 font-medium">
                <MdiRobot class="size-5" />
                {{ $t("profile.integrations.ai_section_title") }}
              </h3>

              <div>
                <Label for="ai-provider"> {{ $t("profile.integrations.provider_label") }} </Label>
                <Select
                  id="ai-provider"
                  :model-value="providerSelectValue"
                  :disabled="!integrationsStore.isOwner"
                  @update:model-value="val => (providerSelectValue = String(val || ''))"
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="opt in providerOptions" :key="opt.value" :value="opt.value">
                      {{ opt.label }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <FormTextField
                v-model="aiBaseUrl"
                :label="$t('profile.integrations.base_url_label')"
                :disabled="!integrationsStore.isOwner"
              />

              <FormTextField
                v-model="aiModel"
                :label="$t('profile.integrations.model_label')"
                :disabled="!integrationsStore.isOwner"
              />

              <div class="flex items-end gap-2">
                <FormPassword
                  v-model="aiApiKeyValue"
                  class="grow"
                  :label="$t('profile.integrations.api_key_label')"
                  :placeholder="aiApiKeyPlaceholder"
                  :disabled="!integrationsStore.isOwner"
                />
                <button
                  v-if="aiApiKeyClearable && integrationsStore.isOwner"
                  type="button"
                  class="mb-2 inline-flex items-center justify-center rounded-md p-2 text-muted-foreground hover:text-foreground"
                  :title="$t('profile.integrations.clear_field')"
                  @click="clearAiApiKey"
                >
                  <MdiClose class="size-4" />
                </button>
              </div>

              <p v-if="envHint" class="text-sm text-muted-foreground">{{ envHint }}</p>

              <div v-if="integrationsStore.isOwner" class="flex items-center gap-2">
                <Button variant="outline" size="sm" :disabled="testingAI" @click="testAI">
                  <MdiLoading v-if="testingAI" class="mr-2 inline-block animate-spin" />
                  <span>{{ $t("profile.integrations.test") }}</span>
                </Button>
                <span v-if="testAIResult" class="flex items-center gap-1 text-sm">
                  <MdiCheck v-if="testAIResult.ok" class="size-4 text-green-500" />
                  <MdiAlertCircleOutline v-else class="size-4 text-destructive" />
                  <span :class="testAIResult.ok ? 'text-green-500' : 'text-destructive'">
                    {{ testAIResult.detail }}
                  </span>
                </span>
              </div>
            </section>

            <section class="space-y-4 border-t pt-4">
              <h3 class="flex items-center gap-2 font-medium">
                <MdiBarcode class="size-5" />
                {{ $t("profile.integrations.upc_section_title") }}
              </h3>

              <div class="flex items-end gap-2">
                <FormPassword
                  v-model="barcodeTokenValue"
                  class="grow"
                  :label="$t('profile.integrations.barcode_token_label')"
                  :placeholder="barcodeTokenPlaceholder"
                  :disabled="!integrationsStore.isOwner"
                />
                <button
                  v-if="barcodeTokenClearable && integrationsStore.isOwner"
                  type="button"
                  class="mb-2 inline-flex items-center justify-center rounded-md p-2 text-muted-foreground hover:text-foreground"
                  :title="$t('profile.integrations.clear_field')"
                  @click="clearBarcodeToken"
                >
                  <MdiClose class="size-4" />
                </button>
              </div>

              <FormTextField
                v-model="openFoodFactsContact"
                :label="$t('profile.integrations.off_contact_label')"
                :disabled="!integrationsStore.isOwner"
              />

              <div v-if="integrationsStore.isOwner" class="flex items-center gap-2">
                <Button variant="outline" size="sm" :disabled="testingBarcode" @click="testBarcode">
                  <MdiLoading v-if="testingBarcode" class="mr-2 inline-block animate-spin" />
                  <span>{{ $t("profile.integrations.test") }}</span>
                </Button>
                <span v-if="testBarcodeResult" class="flex items-center gap-1 text-sm">
                  <MdiCheck v-if="testBarcodeResult.ok" class="size-4 text-green-500" />
                  <MdiAlertCircleOutline v-else class="size-4 text-destructive" />
                  <span :class="testBarcodeResult.ok ? 'text-green-500' : 'text-destructive'">
                    {{ testBarcodeResult.detail }}
                  </span>
                </span>
              </div>
            </section>

            <div v-if="integrationsStore.isOwner" class="mt-4">
              <Button variant="secondary" size="sm" :disabled="savingIntegrations" @click="saveIntegrations">
                <MdiLoading v-if="savingIntegrations" class="mr-2 inline-block animate-spin" />
                <span>{{ $t("profile.integrations.save") }}</span>
              </Button>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
