import { afterEach, describe, expect, test, vi } from "vitest";
import { Requests } from "./requests";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Requests default headers", () => {
  test("resolves dynamic headers for every request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    let tenant = "collection-a";
    const requests = new Requests("/api", "", () => ({ "X-Tenant": tenant }));

    await requests.get<void>({ url: "/entities" });
    tenant = "collection-b";
    await requests.get<void>({ url: "/entities" });

    expect(fetchMock.mock.calls[0]?.[1]?.headers).toMatchObject({ "X-Tenant": "collection-a" });
    expect(fetchMock.mock.calls[1]?.[1]?.headers).toMatchObject({ "X-Tenant": "collection-b" });
  });

  test("lets an explicit request header override a default", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    const requests = new Requests("/api", "", { "X-Tenant": "selected-collection" });
    await requests.get<void>({
      url: "/groups",
      headers: { "X-Tenant": "explicit-collection" },
    });

    expect(fetchMock.mock.calls[0]?.[1]?.headers).toMatchObject({ "X-Tenant": "explicit-collection" });
  });
});
