import type {
  EntityCreate,
  EntityTemplateCreate,
  EntityTemplateUpdate,
  EntityUpdate,
  TagCreate,
  TemplateField,
} from "./data-contracts";

export enum AttachmentTypes {
  Photo = "photo",
  Manual = "manual",
  Warranty = "warranty",
  Attachment = "attachment",
  Receipt = "receipt",
}

export type Result<T> = {
  item: T;
};

export interface PaginationResult<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

/**
 * Go accepts omitted scalar JSON fields as their zero values. Swagger marks
 * every non-pointer field as required, so these request-only adapters model
 * the actual wire contract without weakening response types.
 */
export type EntityCreateInput = Pick<EntityCreate, "name"> & Partial<Omit<EntityCreate, "name">>;
export type EntityUpdateInput = Omit<EntityUpdate, "entityTypeId"> & { entityTypeId?: string };
export type TagCreateInput = Omit<TagCreate, "icon"> & { icon?: string };

export type TemplateFieldInput = Pick<TemplateField, "id" | "name" | "type"> &
  Partial<Omit<TemplateField, "id" | "name" | "type">>;
export type EntityTemplateCreateInput = Omit<EntityTemplateCreate, "fields"> & { fields: TemplateFieldInput[] };
export type EntityTemplateUpdateInput = Omit<EntityTemplateUpdate, "fields"> & { fields: TemplateFieldInput[] };
