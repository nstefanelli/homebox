// Named sheet-label presets. Dimensions in inches on US Letter.
export interface LabelPreset {
  id: string;
  /** i18n key under reports.label_generator.presets.* */
  nameKey: string;
  labelWidth: number;
  labelHeight: number;
  pageWidth: number;
  pageHeight: number;
  pagePaddingTop: number;
  pagePaddingBottom: number;
  pagePaddingLeft: number;
  pagePaddingRight: number;
  /** gap between label columns/rows on the physical sheet (0 = edge-to-edge) */
  gutterX: number;
  gutterY: number;
  measure: "in";
}

export const CUSTOM_PRESET_ID = "custom";

// Geometry verified against the official Avery template PDFs 2026-07-21
// (22806 vertical corrected to template U-0431-01: 0.625in top/bottom margins,
// 2 7/12in vertical pitch). Margins + gutters sum exactly to the page: e.g.
// 5160: 2*0.1875 + 3*2.625 + 2*0.125 = 8.5; 22806 vertically:
// 2*0.625 + 4*2 + 3*(7/12) = 11.
export const LABEL_PRESETS: LabelPreset[] = [
  {
    id: "avery-5160",
    nameKey: "avery_5160",
    labelWidth: 2.625,
    labelHeight: 1,
    pageWidth: 8.5,
    pageHeight: 11,
    pagePaddingTop: 0.5,
    pagePaddingBottom: 0.5,
    pagePaddingLeft: 0.1875,
    pagePaddingRight: 0.1875,
    gutterX: 0.125,
    gutterY: 0,
    measure: "in",
  },
  {
    // Inkjet twin of 5160 — identical geometry, listed separately because users
    // look for the product number printed on their label box.
    id: "avery-8160",
    nameKey: "avery_8160",
    labelWidth: 2.625,
    labelHeight: 1,
    pageWidth: 8.5,
    pageHeight: 11,
    pagePaddingTop: 0.5,
    pagePaddingBottom: 0.5,
    pagePaddingLeft: 0.1875,
    pagePaddingRight: 0.1875,
    gutterX: 0.125,
    gutterY: 0,
    measure: "in",
  },
  {
    id: "avery-5163",
    nameKey: "avery_5163",
    labelWidth: 4,
    labelHeight: 2,
    pageWidth: 8.5,
    pageHeight: 11,
    pagePaddingTop: 0.5,
    pagePaddingBottom: 0.5,
    pagePaddingLeft: 0.15625,
    pagePaddingRight: 0.15625,
    gutterX: 0.1875,
    gutterY: 0,
    measure: "in",
  },
  {
    id: "avery-22806",
    nameKey: "avery_22806",
    labelWidth: 2,
    labelHeight: 2,
    pageWidth: 8.5,
    pageHeight: 11,
    pagePaddingTop: 0.625,
    pagePaddingBottom: 0.625,
    pagePaddingLeft: 0.625,
    pagePaddingRight: 0.625,
    gutterX: 0.625,
    gutterY: 7 / 12, // 7/12 in — vertical pitch 2.58333in minus the 2in label
    measure: "in",
  },
];
