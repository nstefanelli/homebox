import { BaseAPI, route } from "../base";
import type {
  EntityTemplateOut,
  EntityTemplateSummary,
  EntityTemplateCreateItemRequest,
  EntityTemplateBatchCreateRequest,
  EntityOut,
} from "../types/data-contracts";
import type { EntityTemplateCreateInput, EntityTemplateUpdateInput } from "../types/non-generated";

export class TemplatesApi extends BaseAPI {
  getAll() {
    return this.http.get<EntityTemplateSummary[]>({ url: route("/templates") });
  }

  create(body: EntityTemplateCreateInput) {
    return this.http.post<EntityTemplateCreateInput, EntityTemplateOut>({ url: route("/templates"), body });
  }

  get(id: string) {
    return this.http.get<EntityTemplateOut>({ url: route(`/templates/${id}`) });
  }

  delete(id: string) {
    return this.http.delete<void>({ url: route(`/templates/${id}`) });
  }

  update(id: string, body: EntityTemplateUpdateInput) {
    return this.http.put<EntityTemplateUpdateInput, EntityTemplateOut>({ url: route(`/templates/${id}`), body });
  }

  createItem(templateId: string, body: EntityTemplateCreateItemRequest) {
    return this.http.post<EntityTemplateCreateItemRequest, EntityOut>({
      url: route(`/templates/${templateId}/create-item`),
      body,
    });
  }

  batchCreate(templateId: string, body: EntityTemplateBatchCreateRequest) {
    return this.http.post<EntityTemplateBatchCreateRequest, EntityOut[]>({
      url: route(`/templates/${templateId}/batch-create`),
      body,
    });
  }

  uploadPhoto(templateId: string, file: File) {
    const formData = new FormData();
    formData.append("file", file);
    return this.http.post<FormData, EntityTemplateOut>({
      url: route(`/templates/${templateId}/photo`),
      data: formData,
    });
  }

  deletePhoto(templateId: string) {
    return this.http.delete<void>({ url: route(`/templates/${templateId}/photo`) });
  }

  photoUrl(templateId: string): string {
    return this.authURL(`/templates/${templateId}/photo`);
  }
}
