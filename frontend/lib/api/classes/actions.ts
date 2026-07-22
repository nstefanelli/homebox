import { BaseAPI, route } from "../base";
import type { ActionAmountResult, AnalyzePhotoResponse, AnalyzeBulkResponse } from "../types/data-contracts";
import type { IdentifyFromKeywordResponse } from "../types/non-generated";

export class ActionsAPI extends BaseAPI {
  ensureAssetIDs() {
    return this.http.post<void, ActionAmountResult>({
      url: route("/actions/ensure-asset-ids"),
    });
  }

  resetItemDateTimes() {
    return this.http.post<void, ActionAmountResult>({
      url: route("/actions/zero-item-time-fields"),
    });
  }

  ensureImportRefs() {
    return this.http.post<void, ActionAmountResult>({
      url: route("/actions/ensure-import-refs"),
    });
  }

  setPrimaryPhotos() {
    return this.http.post<void, ActionAmountResult>({
      url: route("/actions/set-primary-photos"),
    });
  }

  createMissingThumbnails() {
    return this.http.post<void, ActionAmountResult>({
      url: route("/actions/create-missing-thumbnails"),
    });
  }

  wipeInventory(options?: { wipeTags?: boolean; wipeLocations?: boolean; wipeMaintenance?: boolean }) {
    return this.http.post<
      { wipeTags?: boolean; wipeLocations?: boolean; wipeMaintenance?: boolean },
      ActionAmountResult
    >({
      url: route("/actions/wipe-inventory"),
      body: options || {},
    });
  }

  analyzePhoto(file: File | Blob, signal?: AbortSignal) {
    const formData = new FormData();
    formData.append("file", file);

    return this.http.post<FormData, AnalyzePhotoResponse>({
      url: route("/actions/analyze-photo"),
      data: formData,
      signal,
    });
  }

  /**
   * AI keyword fallback for product lookup: returns ONE BarcodeProduct-shaped
   * candidate flagged aiGuess. Gated exactly like analyze-photo — responds
   * 503 when no AI provider is configured (and 502 on provider failure).
   */
  identifyFromKeyword(keyword: string, signal?: AbortSignal) {
    return this.http.post<{ keyword: string }, IdentifyFromKeywordResponse>({
      url: route("/actions/identify-from-keyword"),
      body: { keyword },
      signal,
    });
  }

  analyzePhotoBulk(file: File | Blob, signal?: AbortSignal) {
    const formData = new FormData();
    formData.append("file", file);

    return this.http.post<FormData, AnalyzeBulkResponse>({
      url: route("/actions/analyze-photo-bulk"),
      data: formData,
      signal,
    });
  }
}
