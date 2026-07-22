import { BaseAPI, route } from "../base";
import type { BarcodeProduct } from "../types/data-contracts";

export class ProductAPI extends BaseAPI {
  searchFromBarcode(productEAN: string, signal?: AbortSignal) {
    return this.http.get<BarcodeProduct[]>({ url: route(`/products/search-from-barcode`, { productEAN }), signal });
  }

  /**
   * Keyword product search (provider-backed, capped server-side). Returns
   * 200 + BarcodeProduct[] on hits, 204 (empty body) when nothing matched,
   * and an error status (502) when every provider failed — callers must
   * treat 204 as "no results", not as a failure.
   */
  searchFromKeyword(keyword: string, signal?: AbortSignal) {
    return this.http.get<BarcodeProduct[]>({ url: route(`/products/search-from-keyword`, { keyword }), signal });
  }
}
