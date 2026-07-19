/**
 * Deterministic post-migration checks for the upgrade-test workflow.
 *
 * The pre-upgrade script stores credentials and stable fixture identifiers,
 * but never authentication tokens. Every user must establish a fresh session
 * against the upgraded application before any persistence checks run.
 */

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { expect, request as playwrightRequest, test } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";

type UserKey = "group1Owner" | "group1Member" | "group2Owner";

interface TestUser {
  key: UserKey;
  email: string;
  password: string;
  group: "group1" | "group2";
  role: "owner" | "member";
}

interface StoredEntity {
  id: string;
  name: string;
  description: string;
  quantity?: number;
}

interface TestData {
  users: TestUser[];
  groups: {
    group1: {
      location: StoredEntity;
      tag: {
        id: string;
        name: string;
        description: string;
        color: string;
      };
      item: StoredEntity & {
        quantity: number;
        locationId: string;
        tagId: string;
      };
      notifier: {
        id: string;
        name: string;
        url: string;
      };
      attachment: {
        id: string;
        entityId: string;
        title: string;
        type: string;
        sha256: string;
      };
    };
    group2: {
      item: StoredEntity & {
        quantity: number;
      };
    };
  };
}

interface LoginResponse {
  token: string;
}

interface UserSelfResponse {
  item: {
    id: string;
    email: string;
    defaultGroupId: string;
    groupIds: string[];
  };
}

interface EntityResponse {
  id: string;
  name: string;
  description: string;
  quantity: number;
  parent?: { id: string; name: string } | null;
  tags: { id: string; name: string }[];
  attachments: {
    id: string;
    title: string;
    type: string;
    mimeType: string;
  }[];
}

interface EntityListResponse {
  items: {
    id: string;
    name: string;
  }[];
}

interface TagResponse {
  id: string;
  name: string;
  description: string;
  color: string;
}

interface NotifierResponse {
  id: string;
  name: string;
  url: string;
  isActive: boolean;
}

interface GroupMemberResponse {
  email: string;
}

const baseURL = process.env.E2E_BASE_URL || "http://localhost:7745";
const testDataPath = process.env.TEST_DATA_FILE || "/tmp/test-users.json";

let testData: TestData;
const apiContexts = new Map<UserKey, APIRequestContext>();
const userProfiles = new Map<UserKey, UserSelfResponse["item"]>();

async function expectJson<T>(response: APIResponse, expectedStatus = 200): Promise<T> {
  expect(response.status()).toBe(expectedStatus);
  return (await response.json()) as T;
}

function user(key: UserKey): TestUser {
  const match = testData.users.find(candidate => candidate.key === key);
  if (!match) {
    throw new Error(`Missing upgrade fixture user: ${key}`);
  }
  return match;
}

function apiContext(key: UserKey): APIRequestContext {
  const context = apiContexts.get(key);
  if (!context) {
    throw new Error(`Missing authenticated API context: ${key}`);
  }
  return context;
}

function profile(key: UserKey): UserSelfResponse["item"] {
  const currentProfile = userProfiles.get(key);
  if (!currentProfile) {
    throw new Error(`Missing upgraded user profile: ${key}`);
  }
  return currentProfile;
}

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  testData = JSON.parse(readFileSync(testDataPath, "utf8")) as TestData;
  expect(testData.users).toHaveLength(3);

  for (const fixtureUser of testData.users) {
    const loginContext = await playwrightRequest.newContext({ baseURL });
    const loginResponse = await loginContext.post("/api/v1/users/login", {
      data: {
        username: fixtureUser.email,
        password: fixtureUser.password,
        stayLoggedIn: false,
      },
    });
    const login = await expectJson<LoginResponse>(loginResponse);
    expect(login.token).toMatch(/^Bearer \S+$/);
    await loginContext.dispose();

    // A new context prevents Homebox session cookies from one user overriding
    // another user's Authorization header.
    const context = await playwrightRequest.newContext({
      baseURL,
      extraHTTPHeaders: { Authorization: login.token },
    });
    apiContexts.set(fixtureUser.key, context);

    const selfResponse = await context.get("/api/v1/users/self");
    const self = await expectJson<UserSelfResponse>(selfResponse);
    userProfiles.set(fixtureUser.key, self.item);
  }
});

test.afterAll(async () => {
  await Promise.all([...apiContexts.values()].map(context => context.dispose()));
});

test("all users can log in and group membership survives", async () => {
  const group1Owner = user("group1Owner");
  const group1Member = user("group1Member");
  const group2Owner = user("group2Owner");

  expect(profile("group1Owner").email).toBe(group1Owner.email);
  expect(profile("group1Member").email).toBe(group1Member.email);
  expect(profile("group2Owner").email).toBe(group2Owner.email);
  expect(profile("group1Member").defaultGroupId).toBe(profile("group1Owner").defaultGroupId);
  expect(profile("group2Owner").defaultGroupId).not.toBe(profile("group1Owner").defaultGroupId);

  const membersResponse = await apiContext("group1Owner").get("/api/v1/groups/members");
  const members = await expectJson<GroupMemberResponse[]>(membersResponse);
  expect(members.map(member => member.email)).toEqual(expect.arrayContaining([group1Owner.email, group1Member.email]));
  expect(members.map(member => member.email)).not.toContain(group2Owner.email);
});

test("shared group entities, location, and tag survive", async () => {
  const { location, tag, item } = testData.groups.group1;
  const memberContext = apiContext("group1Member");

  const locationResponse = await memberContext.get(`/api/v1/entities/${location.id}`);
  const persistedLocation = await expectJson<EntityResponse>(locationResponse);
  expect(persistedLocation).toMatchObject({
    id: location.id,
    name: location.name,
    description: location.description,
  });

  const itemResponse = await memberContext.get(`/api/v1/entities/${item.id}`);
  const persistedItem = await expectJson<EntityResponse>(itemResponse);
  expect(persistedItem).toMatchObject({
    id: item.id,
    name: item.name,
    description: item.description,
    quantity: item.quantity,
  });
  expect(persistedItem.parent?.id).toBe(item.locationId);
  expect(persistedItem.tags.map(persistedTag => persistedTag.id)).toContain(item.tagId);

  const tagsResponse = await memberContext.get("/api/v1/tags");
  const persistedTags = await expectJson<TagResponse[]>(tagsResponse);
  expect(persistedTags).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: tag.id,
        name: tag.name,
        description: tag.description,
        color: tag.color,
      }),
    ])
  );
});

test("owner-only notifier and attachment bytes survive", async () => {
  const { item, notifier, attachment } = testData.groups.group1;
  const ownerContext = apiContext("group1Owner");

  const notifiersResponse = await ownerContext.get("/api/v1/notifiers");
  const persistedNotifiers = await expectJson<NotifierResponse[]>(notifiersResponse);
  expect(persistedNotifiers).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: notifier.id,
        name: notifier.name,
        url: notifier.url,
        isActive: true,
      }),
    ])
  );

  const itemResponse = await ownerContext.get(`/api/v1/entities/${item.id}`);
  const persistedItem = await expectJson<EntityResponse>(itemResponse);
  expect(persistedItem.attachments).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: attachment.id,
        title: attachment.title,
        type: attachment.type,
      }),
    ])
  );

  const downloadResponse = await ownerContext.get(
    `/api/v1/entities/${attachment.entityId}/attachments/${attachment.id}`
  );
  expect(downloadResponse.status()).toBe(200);
  expect(downloadResponse.headers()["content-disposition"]).toContain(attachment.title);
  const downloadedBytes = await downloadResponse.body();
  expect(createHash("sha256").update(downloadedBytes).digest("hex")).toBe(attachment.sha256);
});

test("tenant isolation survives the migration", async () => {
  const group1Item = testData.groups.group1.item;
  const group2Item = testData.groups.group2.item;

  const group1ItemsResponse = await apiContext("group1Member").get(
    "/api/v1/entities?isLocation=false&page=1&pageSize=100"
  );
  const group1Items = await expectJson<EntityListResponse>(group1ItemsResponse);
  expect(group1Items.items.map(item => item.id)).toContain(group1Item.id);
  expect(group1Items.items.map(item => item.id)).not.toContain(group2Item.id);
  expect(group1Items.items.map(item => item.name)).not.toContain(group2Item.name);

  const group2ItemsResponse = await apiContext("group2Owner").get(
    "/api/v1/entities?isLocation=false&page=1&pageSize=100"
  );
  const group2Items = await expectJson<EntityListResponse>(group2ItemsResponse);
  expect(group2Items.items).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: group2Item.id,
        name: group2Item.name,
      }),
    ])
  );
  expect(group2Items.items.map(item => item.id)).not.toContain(group1Item.id);
  expect(group2Items.items.map(item => item.name)).not.toContain(group1Item.name);
});

test("an upgraded account can log in through the browser", async ({ page }) => {
  const group1Owner = user("group1Owner");

  await page.goto("/home");
  await expect(page).toHaveURL("/");
  await expect(page.locator("#login-form")).toBeVisible();
  await page.locator("#login-username").fill(group1Owner.email);
  await page.locator("#login-password").fill(group1Owner.password);
  await page.locator('#login-form button[type="submit"]').click();

  await expect(page).toHaveURL("/home");
  await expect(page.getByTestId("logout-button")).toBeVisible();
});
