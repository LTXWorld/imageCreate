import { afterEach, describe, expect, test, vi } from "vitest";

import { api, createGeneration, normalizeAuthResponse } from "./client";

function jsonResponse(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

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
      expect.objectContaining({ credentials: "include" }),
    );

    const [, init] = fetchMock.mock.calls[0];
    const headers = new Headers(init?.headers);

    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Request-ID")).toBe("request-1");
  });

  test("normalizes Headers init while preserving JSON content type", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

    await api<{ ok: boolean }>("/api/example", {
      headers: new Headers([["X-Trace-ID", "trace-1"]]),
    });

    const [, init] = fetchMock.mock.calls[0];
    const headers = new Headers(init?.headers);

    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Trace-ID")).toBe("trace-1");
  });

  test("accepts empty 204 responses", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));

    await expect(api<void>("/api/example", { method: "DELETE" })).resolves.toBeUndefined();
  });

  test("createGeneration sends multipart form data when reference image is provided", async () => {
    const file = new File(["reference-bytes"], "reference.png", { type: "image/png" });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      await jsonResponse({
        task: {
          id: "task-reference",
          prompt: "电影海报",
          ratio: "1:1",
          size: "1024x1024",
          status: "queued",
          created_at: "2026-04-30T08:00:00Z",
        },
      }),
    );

    await createGeneration({ prompt: "电影海报", ratio: "1:1", referenceImage: file });

    const [, init] = fetchMock.mock.calls[0];
    expect(init).toMatchObject({ method: "POST", credentials: "include" });
    expect(init?.body).toBeInstanceOf(FormData);
    const headers = new Headers(init?.headers);
    expect(headers.has("Content-Type")).toBe(false);
    const body = init?.body as FormData;
    expect(body.get("prompt")).toBe("电影海报");
    expect(body.get("ratio")).toBe("1:1");
    expect(body.get("reference_image")).toBe(file);
  });

  test("normalizes split credit wallet fields", () => {
    const { user } = normalizeAuthResponse({
      user: {
        id: "user-1",
        username: "alice",
        role: "user",
        status: "active",
        credit_balance: 7,
        daily_free_credit_limit: 5,
        daily_free_credit_balance: 2,
        paid_credit_balance: 5,
      },
    });

    expect(user.creditBalance).toBe(7);
    expect(user.dailyFreeCreditLimit).toBe(5);
    expect(user.dailyFreeCreditBalance).toBe(2);
    expect(user.paidCreditBalance).toBe(5);
  });

  test("derives split wallet fields from legacy credit balance", () => {
    const { user } = normalizeAuthResponse({
      user: {
        id: "user-1",
        username: "alice",
        role: "user",
        status: "active",
        credit_balance: 5,
      },
    });

    expect(user.creditBalance).toBe(5);
    expect(user.dailyFreeCreditLimit).toBe(5);
    expect(user.dailyFreeCreditBalance).toBe(5);
    expect(user.paidCreditBalance).toBe(0);
  });
});
