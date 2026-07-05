export const CONTAINER_DEFAULT_ICON = "package-variant";
export const LOCATION_DEFAULT_ICON = "map-marker-outline";

export type EntityIconSource = {
  icon?: string | null;
  typeIcon?: string | null;
  isContainer?: boolean;
  isLocation?: boolean;
};

/**
 * Resolves which registry icon name to render for a location/container:
 * entity override -> entity-type icon -> flavor default. Unknown names are
 * treated as unset so a stale value can never break rendering.
 */
export function resolveEntityIconName(src: EntityIconSource, knownNames: readonly string[]): string {
  for (const candidate of [src.icon, src.typeIcon]) {
    if (candidate && knownNames.includes(candidate)) {
      return candidate;
    }
  }
  return src.isContainer ? CONTAINER_DEFAULT_ICON : LOCATION_DEFAULT_ICON;
}
