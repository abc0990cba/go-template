import { readFileSync } from "node:fs";
import path from "node:path";
import Ajv2020 from "ajv/dist/2020";
import addFormats from "ajv-formats";
import { describe, expect, it } from "vitest";
import { parse } from "yaml";
import { handlers } from "./handlers";

/**
 * The mock handlers are the SPA's executable reading of the backend
 * contract; this gate keeps them honest against api/openapi/openapi.yaml —
 * the same drift philosophy as the codegen job, applied to the frontend
 * mocks. Two levels:
 *   1. path+method — a handler the spec does not define fails here;
 *   2. shape — every handler's representative response is validated with
 *      ajv (JSON Schema 2020-12, which OpenAPI 3.1 schemas are) against the
 *      operation's declared response, so fixture drift fails in the ordinary
 *      `make web-test` run instead of hiding a wire-shape mismatch.
 */

const specPath = path.resolve(import.meta.dirname, "../../../../api/openapi/openapi.yaml");

interface JsonSchema {
  $ref?: string;
  [key: string]: unknown;
}

interface Operation {
  responses?: Record<string, { content?: Record<string, { schema?: JsonSchema }> }>;
  [method: string]: unknown;
}

interface Spec {
  paths: Record<string, Record<string, Operation>>;
  components?: { schemas?: Record<string, JsonSchema> };
}

function loadSpec(): Spec {
  return parse(readFileSync(specPath, "utf8")) as Spec;
}

/** MSW's ":param" segments are OpenAPI's "{param}". */
function normalizeTemplate(mswPath: string): string {
  return mswPath.replace(/:([A-Za-z0-9_]+)/g, "{$1}");
}

/** MSW path templates carry no type info; the probe value only has to match
 * the segment so MSW's matcher fills the params. */
function probePath(mswPath: string): string {
  return mswPath.replace(/:[A-Za-z0-9_]+/g, "contract-probe");
}

/** Inline $refs into a copy of the schema. The contract has no recursive
 * schemas, so a depth guard is enough to stay safe. */
function resolveRef(schema: JsonSchema, spec: Spec, depth = 0): JsonSchema {
  if (depth > 12 || typeof schema !== "object") return schema;
  if (schema.$ref) {
    const target = schema.$ref
      .replace(/^#\//, "")
      .split("/")
      .map((part) => part.replace(/~1/g, "/").replace(/~0/g, "~"));
    let node: unknown = spec;
    for (const key of target) {
      node = (node as Record<string, unknown>)[key];
    }
    return resolveRef(node as JsonSchema, spec, depth + 1);
  }
  const out: JsonSchema = {};
  for (const [key, value] of Object.entries(schema)) {
    if (value !== null && typeof value === "object") {
      out[key] = Array.isArray(value)
        ? value.map((item) =>
            item !== null && typeof item === "object" ? resolveRef(item, spec, depth + 1) : item,
          )
        : resolveRef(value as JsonSchema, spec, depth + 1);
    } else {
      out[key] = value;
    }
  }
  return out;
}

// strict: false — the schemas carry OpenAPI vocabulary (examples, summaries)
// that JSON Schema 2020-12 validation should ignore, not reject.
const ajv = new Ajv2020({ allErrors: true, strict: false });
addFormats(ajv);

function specOperations(spec: Spec): Map<string, Operation> {
  const operations = new Map<string, Operation>();
  for (const [route, methods] of Object.entries(spec.paths ?? {})) {
    for (const [method, operation] of Object.entries(methods)) {
      if (method === "parameters" || method.startsWith("x-")) continue;
      operations.set(`${method.toUpperCase()} ${route}`, operation as Operation);
    }
  }
  return operations;
}

describe("MSW handlers vs OpenAPI spec", () => {
  const spec = loadSpec();
  const operations = specOperations(spec);

  it("only mocks operations the API contract defines", () => {
    const drifted: string[] = [];
    for (const handler of handlers) {
      const method = String(handler.info.method).toUpperCase();
      const operation = `${method} ${normalizeTemplate(String(handler.info.path))}`;
      if (!operations.has(operation)) drifted.push(operation);
    }
    expect(
      drifted,
      `handlers mock operations missing from ${path.relative(process.cwd(), specPath)} — ` +
        "fix the handler or update the spec",
    ).toEqual([]);
  });

  it("returns responses shaped like the contract declares", async () => {
    const drifted: string[] = [];
    for (const handler of handlers) {
      const method = String(handler.info.method).toUpperCase();
      const template = `${method} ${normalizeTemplate(String(handler.info.path))}`;
      const operation = operations.get(template);
      if (!operation) continue; // covered by the path gate above

      const request = new Request(`http://contract.probe${probePath(String(handler.info.path))}`, {
        method,
      });
      const result = (await handler.run({
        request,
        requestId: "contract-shape-probe",
        // Relative handler paths match against this origin, the same way
        // the MSW worker resolves them in the browser.
        resolutionContext: { baseUrl: "http://contract.probe" },
      })) as {
        response?: Response;
      } | null;
      const response = result?.response;
      if (!response) continue;

      const status = String(response.status);
      const declared =
        operation.responses?.[status]?.content?.["application/json"]?.schema ??
        (response.status >= 400
          ? operation.responses?.default?.content?.["application/json"]?.schema
          : undefined);
      if (!declared) continue;

      const contentType = response.headers.get("content-type") ?? "";
      if (!contentType.includes("json")) continue;
      const body: unknown = await response.json();

      const valid = ajv.validate(resolveRef(declared, spec), body);
      if (!valid) {
        drifted.push(
          `${template} ${status}: ${ajv.errors?.map((e) => `${e.instancePath} ${e.message}`).join("; ")}`,
        );
      }
    }
    expect(
      drifted,
      "handler fixtures drift from the contract's response schemas — fix the fixture or the spec",
    ).toEqual([]);
  });
});
