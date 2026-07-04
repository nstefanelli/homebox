export interface CatalogEntry {
  name: string;
  capacity: string;
  dimensions: string; // exterior L × W × H
  color: string;
}

export const CONTAINER_TYPE_NAME = "Tote";

// Dimensions verified against retailer/manufacturer product pages 2026-07-04
// (see docs/plans/data/ note in Task 13; "~" marks approximate/unverified).
export const CONTAINER_CATALOG: CatalogEntry[] = [
  {
    name: "HDX 17 Gal. Tough Storage Tote",
    capacity: "17 gal",
    dimensions: '26.7" × 17.8" × 12.5"',
    color: "Black/Yellow Lid",
  },
  {
    name: "HDX 27 Gal. Tough Storage Tote",
    capacity: "27 gal",
    dimensions: '28.55" × 19.61" × 15.27"',
    color: "Black/Yellow",
  },
  {
    name: "HDX 38 Gal. Tough Storage Tote",
    capacity: "38 gal",
    dimensions: '38.1" × 21.9" × 15.5"',
    color: "Black/Yellow Lid",
  },
  {
    name: "HDX 55 Gal. Tough Storage Tote",
    capacity: "55 gal",
    dimensions: '~45" × 20" × 20"',
    color: "Black/Yellow Lid",
  },
  {
    name: "Husky 12 Gal. Heavy Duty Waterproof Container",
    capacity: "12 gal",
    dimensions: '~22" × 16" × 10"',
    color: "Red",
  },
  {
    name: "Sterilite 64 Qt. Latching Box",
    capacity: "64 qt",
    dimensions: '23.75" × 16" × 13.5"',
    color: "Clear/White Lid",
  },
  {
    name: "Sterilite 106 Qt. Latching Box",
    capacity: "106 qt",
    dimensions: '33.875" × 18.75" × 13"',
    color: "Clear/White Lid",
  },
  {
    name: "Sterilite 27 Qt. ClearView Latch Box",
    capacity: "27 qt",
    dimensions: '17" × 11.125" × 12.75"',
    color: "Clear",
  },
  { name: "IKEA SAMLA 45 L", capacity: "45 L", dimensions: '22.5" × 15.25" × 11"', color: "Transparent" },
  { name: "IKEA SAMLA 65 L", capacity: "65 L", dimensions: '22.5" × 15.25" × 16.5"', color: "Transparent" },
  { name: "Milk Crate", capacity: "~16 qt", dimensions: '13" × 13" × 11"', color: "Varies" },
  { name: "Banker's Box Stor/File", capacity: "~30 L", dimensions: '15" × 12" × 10"', color: "White/Blue" },
];

/** Template custom fields for a catalog entry. */
export function catalogFields(entry: CatalogEntry): { type: "text"; name: string; textValue: string }[] {
  return [
    { type: "text", name: "Capacity", textValue: entry.capacity },
    { type: "text", name: "Dimensions", textValue: entry.dimensions },
    { type: "text", name: "Color", textValue: entry.color },
  ];
}
