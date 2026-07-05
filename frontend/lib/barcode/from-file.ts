import { BarcodeDetector } from "barcode-detector";

/** Product-code formats the UPC lookup pipeline accepts. */
const PRODUCT_BARCODE_FORMATS = ["ean_13", "ean_8", "upc_a", "upc_e"] as const;

/**
 * Attempts to decode a product barcode from a still photo. Returns the raw
 * barcode value, or null when no barcode is found OR detection is
 * unavailable/fails — callers fall through to the vision lane on null.
 */
export async function detectProductBarcode(file: File): Promise<string | null> {
  try {
    const detector = new BarcodeDetector({ formats: [...PRODUCT_BARCODE_FORMATS] });
    const bitmap = await createImageBitmap(file);
    try {
      const results = await detector.detect(bitmap);
      const first = results[0];
      return first?.rawValue ? first.rawValue : null;
    } finally {
      bitmap.close();
    }
  } catch {
    return null;
  }
}
