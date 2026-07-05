import { BaseAPI, route } from "../base";
import type { BarcodeProduct } from "../types/data-contracts";

export class ProductAPI extends BaseAPI {
  searchFromBarcode(productEAN: string, signal?: AbortSignal) {
    return this.http.get<BarcodeProduct[]>({ url: route(`/products/search-from-barcode`, { productEAN }), signal });
  }
}
