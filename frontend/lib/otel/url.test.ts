import { describe, expect, test } from "vitest";
import { sanitizeTraceURL } from "./url";

describe("sanitizeTraceURL", () => {
  test("removes attachment credentials, tenant ids, and fragments from absolute URLs", () => {
    const result = sanitizeTraceURL(
      "https://homebox.example/api/v1/entities/abc/attachments/def?access_token=secret&tenant=private-group#preview"
    );

    expect(result).toBe("https://homebox.example/api/v1/entities/abc/attachments/def");
    expect(result).not.toContain("secret");
    expect(result).not.toContain("private-group");
  });

  test("keeps only the path for relative application URLs", () => {
    expect(sanitizeTraceURL("/api/v1/entities?field=serial#results")).toBe("/api/v1/entities");
  });

  test("does not preserve malformed input", () => {
    expect(sanitizeTraceURL("https://%")).toBe("[invalid-url]");
  });
});
