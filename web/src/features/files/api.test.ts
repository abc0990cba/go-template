import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { fileFixture, problem } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";

import {
  abortMultipart,
  initiateMultipart,
  multipartCapability,
  putMultipartPart,
  uploadAttachment,
  uploadSingleShot,
} from "./api";

/** The resumable-upload orchestrator over MSW: transport selection, the
 * streaming part loop, and the presigned-PUT variant. */
describe("uploadAttachment", () => {
  it("takes the single-shot path for small files", async () => {
    const stored = await uploadAttachment(
      new File(["hello"], "small.txt", { type: "text/plain" }),
      {
        type: "ticket",
        id: "t-1",
      },
    );
    expect(stored.filename).toBe("evidence.bin");
  });

  it("streams parts and completes when sessions are enabled", async () => {
    // 17 bytes over a tiny part size? The part size is a module constant
    // (8 MiB), so a small file stays single-shot; drive the multipart path
    // by making the capability claim enabled AND posting a big-enough blob.
    const big = new File([new ArrayBuffer(17 * 1024 * 1024)], "report.bin", {
      type: "application/octet-stream",
    });
    const etags: string[] = [];
    server.use(
      http.post("/api/files/multipart/parts", () => {
        const etag = `"part-${etags.length + 1}"`;
        etags.push(etag);
        return new HttpResponse(null, { status: 204, headers: { ETag: etag } });
      }),
    );

    const stored = await uploadAttachment(big, { type: "ticket", id: "t-1" });
    expect(stored.filename).toBe("report.bin");
    // 17 MiB over 8 MiB parts = 3 parts, each carrying its ETag.
    expect(etags).toHaveLength(3);
  });

  it("uses presigned part URLs when the session is not streaming", async () => {
    const uploadedUrls: string[] = [];
    server.use(
      http.get("/api/files/multipart", () =>
        HttpResponse.json({ enabled: true, streaming: false, max_size: 1 << 30 }),
      ),
      http.post("/api/files/multipart", () =>
        HttpResponse.json(
          {
            key: "k-1/big.bin",
            upload_id: "upl-9",
            streaming: false,
            part_urls: ["https://storage.example/part1", "https://storage.example/part2"],
          },
          { status: 201 },
        ),
      ),
      http.put("https://storage.example/part1", () => {
        uploadedUrls.push("part1");
        return new HttpResponse(null, { status: 200, headers: { ETag: '"etag-1"' } });
      }),
      http.put("https://storage.example/part2", () => {
        uploadedUrls.push("part2");
        return new HttpResponse(null, { status: 200, headers: { ETag: '"etag-2"' } });
      }),
    );

    // 16 MiB over 8 MiB parts = exactly two parts, matching the two URLs.
    const large = new File([new ArrayBuffer(16 * 1024 * 1024)], "big.bin");
    const stored = await uploadAttachment(large, { type: "ticket", id: "t-1" });
    expect(stored.id).toBe("f-1");
    expect(uploadedUrls).toEqual(["part1", "part2"]);
  });

  it("degrades to single-shot when the capability endpoint fails", async () => {
    server.use(http.get("/api/files/multipart", () => problem(500, "internal error", "")));
    const big = new File([new ArrayBuffer(17 * 1024 * 1024)], "report.bin");
    const stored = await uploadAttachment(big, { type: "ticket", id: "t-1" });
    expect(stored.filename).toBe("evidence.bin");
  });

  it("exposes the session primitives the orchestrator composes", async () => {
    const capability = await multipartCapability();
    expect(capability.enabled).toBe(true);
    const session = await initiateMultipart(new File(["x"], "f.bin"), 2, {
      type: "ticket",
      id: "t-1",
    });
    expect(session.upload_id).toBe("upl-1");
    const etag = await putMultipartPart(session.upload_id, 1, new Blob(["x"]));
    expect(etag).toBe('"part-etag"');
    await expect(
      abortMultipart({ key: session.key, upload_id: session.upload_id }),
    ).resolves.toBeUndefined();
    void fileFixture;
  });

  it("uploads single-shot files through the typed client", async () => {
    const stored = await uploadSingleShot(new File(["x"], "note.txt", { type: "text/plain" }), {
      type: "ticket",
      id: "t-1",
    });
    expect(stored.filename).toBe("evidence.bin");
  });
});
