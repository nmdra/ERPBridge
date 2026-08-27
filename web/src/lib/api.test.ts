import { afterEach, expect, test, vi } from "vitest";

import { apiFetch, ConsoleAPIError } from "./api";

afterEach(() => vi.restoreAllMocks());

test("preserves safe BFF error code and message", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            error: "context_not_found",
            message: "the selected context is not configured",
          }),
          { status: 404 },
        ),
      ),
    ),
  );

  await expect(apiFetch("/contexts")).rejects.toMatchObject({
    name: "ConsoleAPIError",
    code: "context_not_found",
    status: 404,
    message: "the selected context is not configured",
  });
});

test("does not expose an unstructured response body", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(new Response("secret upstream body", { status: 502 })),
    ),
  );

  const error = await apiFetch("/contexts").catch((value: unknown) => value);
  expect(error).toBeInstanceOf(ConsoleAPIError);
  expect((error as Error).message).toBe(
    "Console request failed with status 502",
  );
  expect((error as Error).message).not.toContain("secret upstream body");
});
