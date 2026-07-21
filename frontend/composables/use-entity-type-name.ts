import { useI18n } from "vue-i18n";

/**
 * Resolves an entity type's display name.
 *
 * The seeded default entity types store i18n keys as their name (the
 * migrations insert "global.item" / "global.location" literally), while
 * user-created types store literal names (e.g. "Tote"). Translate only when
 * the name resolves to a known message key — also checking the fallback
 * locale ("en"), since `te` only inspects the active locale — so literal
 * user names pass through untouched and never trigger vue-i18n
 * missing-key warnings.
 */
export function translateEntityTypeName(
  name: string | null | undefined,
  t: (key: string) => string,
  te: (key: string, locale?: string) => boolean
): string {
  if (!name) return "";
  return te(name) || te(name, "en") ? t(name) : name;
}

/**
 * Composable wrapper around {@link translateEntityTypeName} bound to the
 * active i18n instance. Usage: `const typeName = useEntityTypeName();`
 * then `typeName(type.name)` in templates or script.
 */
export function useEntityTypeName(): (name: string | null | undefined) => string {
  const { t, te } = useI18n();
  return name => translateEntityTypeName(name, t, te);
}
