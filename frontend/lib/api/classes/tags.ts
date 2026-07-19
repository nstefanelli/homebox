import { BaseAPI, route } from "../base";
import type { TagOut } from "../types/data-contracts";
import type { TagCreateInput } from "../types/non-generated";

export class TagsApi extends BaseAPI {
  getAll() {
    return this.http.get<TagOut[]>({ url: route("/tags") });
  }

  create(body: TagCreateInput) {
    return this.http.post<TagCreateInput, TagOut>({ url: route("/tags"), body });
  }

  get(id: string) {
    return this.http.get<TagOut>({ url: route(`/tags/${id}`) });
  }

  delete(id: string) {
    return this.http.delete<void>({ url: route(`/tags/${id}`) });
  }

  update(id: string, body: TagCreateInput) {
    return this.http.put<TagCreateInput, TagOut>({ url: route(`/tags/${id}`), body });
  }
}
