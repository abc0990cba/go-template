import { http, HttpResponse } from "msw";

import { fileFixture } from "../fixtures";

/** The files slice: single-shot uploads, resumable sessions, and the
 * ticket-attachments listing. Part uploads answer an ETag header like the
 * real backend, so the client's completion payload round-trips. */
export const files = [
  http.get("/api/files/multipart", () =>
    HttpResponse.json({ enabled: true, streaming: true, max_size: 10 * 1024 * 1024 }),
  ),
  http.post("/api/files/multipart", () =>
    HttpResponse.json(
      { key: "k-1/report.bin", upload_id: "upl-1", streaming: true },
      { status: 201 },
    ),
  ),
  http.post(
    "/api/files/multipart/parts",
    () => new HttpResponse(null, { status: 204, headers: { ETag: '"part-etag"' } }),
  ),
  http.post("/api/files/multipart/complete", () =>
    HttpResponse.json(fileFixture({ filename: "report.bin", size_bytes: 17 }), { status: 201 }),
  ),
  http.post("/api/files/multipart/abort", () => new HttpResponse(null, { status: 204 })),
  http.post("/api/files", () => HttpResponse.json(fileFixture(), { status: 201 })),
  http.get("/api/tickets/:id/attachments", () => HttpResponse.json({ data: [fileFixture()] })),
  http.get("/api/files/:id/content", () => new HttpResponse("hello world", { status: 200 })),
];
