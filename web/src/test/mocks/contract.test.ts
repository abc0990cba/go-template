import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { parse } from "yaml";
import { handlers } from "./handlers";

/**
 * The mock handlers are the SPA's executable reading of the backend
 * contract; this gate keeps them honest against api/openapi/openapi.yaml —
 * the same drift philosophy as the codegen job, applied to the frontend
 * mocks. A handler whose path+method the spec does not define fails here,
 * in the ordinary `make web-test` run, instead of silently mocking an
 * endpoint that no longer exists.
 */

const specPath = path.resolve(import.meta.dirname, "../../../../api/openapi/openapi.yaml");

interface Operation {
  [method: string]: unknown;
}

function specOperations(): Set<string> {
  const spec = parse(readFileSync(specPath, "utf8")) as {
    paths: Record<string, Record<string, Operation>>;
  };
  const operations = new Set<string>();
  for (const [route, methods] of Object.entries(spec.paths ?? {})) {
    for (const method of Object.keys(methods)) {
      if (method === "parameters" || method.startsWith("x-")) continue;
      operations.add(`${method.toUpperCase()} ${route}`);
    }
  }
  return operations;
}

/** MSW's ":param" segments are OpenAPI's "{param}". */
function normalizeTemplate(mswPath: string): string {
  return mswPath.replace(/:([A-Za-z0-9_]+)/g, "{$1}");
}

describe("MSW handlers vs OpenAPI spec", () => {
  it("only mocks operations the API contract defines", () => {
    const operations = specOperations();
    const drifted: string[] = [];

    for (const handler of handlers) {
      const { method, path: rawPath } = handler.info;
      if (typeof rawPath !== "string") {
        drifted.push(`${method} ${String(rawPath)} (non-template path)`);
        continue;
      }
      const operation = `${method.toUpperCase()} ${normalizeTemplate(rawPath)}`;
      if (!operations.has(operation)) drifted.push(operation);
    }

    expect(
      drifted,
      `handlers mock operations missing from ${path.relative(process.cwd(), specPath)} — ` +
        "fix the handler or update the spec",
    ).toEqual([]);
  });
});
