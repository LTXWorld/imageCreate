import { afterEach, describe, expect, test, vi } from "vitest";

import { api } from "./client";

describe("api", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("preserves include credentials and JSON content type", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

    await api<{ ok: boolean }>("/api/example", {
      credentials: "omit",
      headers: { "X-Request-ID": "request-1" },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/example",
      expect.objectContaining({
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          "X-Request-ID": "request-1",
        },
      }),
    );
  });
});
