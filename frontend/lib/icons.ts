import MdiTagOutline from "~icons/mdi/tag-outline";
import MdiTreeOutline from "~icons/mdi/tree-outline";
import MdiBagSuitcaseOutline from "~icons/mdi/bag-suitcase-outline";
import MdiBedOutline from "~icons/mdi/bed-outline";
import MdiKitchenCounterOutline from "~icons/mdi/kitchen-counter-outline";
import MdiBookOpenVariantOutline from "~icons/mdi/book-open-variant-outline";
import MdiLaptopOutline from "~icons/mdi/laptop";
import MdiToolboxOutline from "~icons/mdi/toolbox-outline";
import MdiFileCabinetOutline from "~icons/mdi/folder-outline";
import MdiDresserOutline from "~icons/mdi/dresser-outline";
import MdiLightbulbOutline from "~icons/mdi/lightbulb-outline";
import MdiPowerPlugOutline from "~icons/mdi/power-plug-outline";
import MdiWrenchOutline from "~icons/mdi/wrench-outline";
import MdiDumbbellOutline from "~icons/mdi/dumbbell";
import MdiSofaOutline from "~icons/mdi/sofa-outline";
import MdiPalleteOutline from "~icons/mdi/palette-outline";
import MdiMapMarkerOutline from "~icons/mdi/map-marker-outline";
import MdiPackageVariant from "~icons/mdi/package-variant";
import MdiPackageVariantClosed from "~icons/mdi/package-variant-closed";
import MdiBasketOutline from "~icons/mdi/basket-outline";
import MdiBookshelf from "~icons/mdi/bookshelf";
import MdiGarage from "~icons/mdi/garage";
import MdiWardrobeOutline from "~icons/mdi/wardrobe-outline";
import MdiStairsDown from "~icons/mdi/stairs-down";
import MdiHomeRoof from "~icons/mdi/home-roof";
import MdiDoor from "~icons/mdi/door";
import MdiArchiveOutline from "~icons/mdi/archive-outline";
import MdiCubeOutline from "~icons/mdi/cube-outline";
import MdiFridgeOutline from "~icons/mdi/fridge-outline";
import MdiSafe from "~icons/mdi/safe";
import MdiHanger from "~icons/mdi/hanger";
import MdiTreasureChest from "~icons/mdi/treasure-chest";
import { resolveEntityIconName, type EntityIconSource } from "./entity-icon";

export const availableIcons = [
  { name: "tag-outline", component: MdiTagOutline },
  { name: "tree-outline", component: MdiTreeOutline },
  { name: "bag-suitcase-outline", component: MdiBagSuitcaseOutline },
  { name: "bed-outline", component: MdiBedOutline },
  { name: "kitchen-counter-outline", component: MdiKitchenCounterOutline },
  { name: "book-open-variant-outline", component: MdiBookOpenVariantOutline },
  { name: "laptop", component: MdiLaptopOutline },
  { name: "sofa-outline", component: MdiSofaOutline },
  { name: "toolbox-outline", component: MdiToolboxOutline },
  { name: "file-cabinet-outline", component: MdiFileCabinetOutline },
  { name: "dresser-outline", component: MdiDresserOutline },
  { name: "lightbulb-outline", component: MdiLightbulbOutline },
  { name: "power-plug-outline", component: MdiPowerPlugOutline },
  { name: "wrench-outline", component: MdiWrenchOutline },
  { name: "dumbbell", component: MdiDumbbellOutline },
  { name: "palette-outline", component: MdiPalleteOutline },
  { name: "map-marker-outline", component: MdiMapMarkerOutline },
  { name: "package-variant", component: MdiPackageVariant },
  { name: "package-variant-closed", component: MdiPackageVariantClosed },
  { name: "basket-outline", component: MdiBasketOutline },
  { name: "bookshelf", component: MdiBookshelf },
  { name: "garage", component: MdiGarage },
  { name: "wardrobe-outline", component: MdiWardrobeOutline },
  { name: "stairs-down", component: MdiStairsDown },
  { name: "home-roof", component: MdiHomeRoof },
  { name: "door", component: MdiDoor },
  { name: "archive-outline", component: MdiArchiveOutline },
  { name: "cube-outline", component: MdiCubeOutline },
  { name: "fridge-outline", component: MdiFridgeOutline },
  { name: "safe", component: MdiSafe },
  { name: "hanger", component: MdiHanger },
  { name: "treasure-chest", component: MdiTreasureChest },
] as const;

export type IconName = (typeof availableIcons)[number]["name"];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function getIconComponent(iconName: string | undefined): any {
  if (!iconName) {
    return defaultIcon;
  }
  const icon = availableIcons.find(i => i.name === iconName);
  return icon ? icon.component : defaultIcon;
}

export const defaultIcon = MdiTagOutline;

const iconNames = availableIcons.map(i => i.name) as readonly string[];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function resolveEntityIcon(src: EntityIconSource): any {
  const name = resolveEntityIconName(src, iconNames);
  return availableIcons.find(i => i.name === name)!.component;
}
