import { api, presignedPut, unwrap, unwrapWithETag } from "@/shared/api/core";
import type { components, operations } from "@/shared/api/schema.gen";

/** Endpoints of the files slice: single-shot uploads and the resumable
 * (multipart) sessions for large attachments. Small files take the
 * single-shot path; anything big enough to need two parts goes through a
 * session — parts stream through the API in dev mode or go straight to
 * presigned part URLs in S3 mode. Every call goes through the typed
 * client; the one exception (presigned storage PUTs) lives in core. */

export type StoredFile = components["schemas"]["File"];
export type MultipartCapability =
  operations["multipartCapability"]["responses"][200]["content"]["application/json"];
export type MultipartSession =
  operations["initiateMultipart"]["responses"][201]["content"]["application/json"];

/** Attachment reference the backend permission-checks. */
export interface AttachmentRef {
  type: "ticket";
  id: string;
}

/** Part size for resumable uploads: 8 MiB, the S3-friendly default. */
export const MULTIPART_PART_SIZE = 8 * 1024 * 1024;

/** listTicketAttachments returns a ticket's files, newest first. */
export function listTicketAttachments(ticketId: string): Promise<StoredFile[]> {
  return unwrap(
    api.GET("/api/tickets/{id}/attachments", { params: { path: { id: ticketId } } }),
  ).then((page) => page.data);
}

export function multipartCapability(): Promise<MultipartCapability> {
  return unwrap(api.GET("/api/files/multipart"));
}

export function uploadSingleShot(file: globalThis.File, ref?: AttachmentRef): Promise<StoredFile> {
  // Multipart/form-data with a real Blob: the generated type names `file`
  // as a string (OpenAPI's binary quirk), so the FormData rides through a
  // boundary cast — openapi-fetch forwards it untouched.
  const body = new FormData();
  body.set("file", file);
  if (ref) {
    body.set("ref_type", ref.type);
    body.set("ref_id", ref.id);
  }
  return unwrap(api.POST("/api/files", { body: body as unknown as { file: string } }));
}

export function initiateMultipart(
  file: globalThis.File,
  parts: number,
  ref?: AttachmentRef,
): Promise<MultipartSession> {
  return unwrap(
    api.POST("/api/files/multipart", {
      body: {
        filename: file.name,
        content_type: file.type || "application/octet-stream",
        parts,
        ref_type: ref?.type,
        ref_id: ref?.id,
      },
    }),
  );
}

/** putMultipartPart streams one part's bytes through the API (dev mode,
 * where the in-process store has no presigned URLs) and returns its ETag. */
export async function putMultipartPart(
  uploadId: string,
  partNumber: number,
  blob: Blob,
): Promise<string> {
  const { etag } = await unwrapWithETag(
    api.POST("/api/files/multipart/parts", {
      params: { query: { upload_id: uploadId, part_number: partNumber } },
      // The wire format is raw bytes; the generated type names a string
      // because OpenAPI has no binary query for "*/*" bodies.
      body: blob as unknown as string,
    }),
  );
  return etag;
}

export async function completeMultipart(
  file: globalThis.File,
  session: MultipartSession,
  parts: Array<{ part_number: number; etag: string }>,
  ref?: AttachmentRef,
): Promise<StoredFile> {
  return unwrap(
    api.POST("/api/files/multipart/complete", {
      body: {
        filename: file.name,
        content_type: file.type || "application/octet-stream",
        key: session.key,
        upload_id: session.upload_id,
        ref_type: ref?.type,
        ref_id: ref?.id,
        parts,
      },
    }),
  );
}

export function abortMultipart(
  session: Pick<MultipartSession, "key" | "upload_id">,
): Promise<void> {
  return unwrap(
    api.POST("/api/files/multipart/abort", {
      body: { key: session.key, upload_id: session.upload_id },
    }),
  );
}

export interface UploadProgress {
  done: number;
  total: number;
}

/**
 * uploadAttachment picks the right transport for one file: single-shot for
 * small files (and when sessions are unavailable), a resumable session for
 * anything that needs two or more parts. onProgress reports completed
 * parts, not bytes.
 */
export async function uploadAttachment(
  file: globalThis.File,
  ref?: AttachmentRef,
  onProgress?: (progress: UploadProgress) => void,
): Promise<StoredFile> {
  const parts = Math.ceil(file.size / MULTIPART_PART_SIZE);

  let capability: MultipartCapability | null = null;
  if (parts >= 2) {
    try {
      capability = await multipartCapability();
    } catch {
      capability = null; // degrade to single-shot; the size cap decides
    }
  }

  if (!capability || !capability.enabled || parts < 2) {
    onProgress?.({ done: 0, total: 1 });
    const stored = await uploadSingleShot(file, ref);
    onProgress?.({ done: 1, total: 1 });
    return stored;
  }

  const session = await initiateMultipart(file, parts, ref);
  const uploaded: Array<{ part_number: number; etag: string }> = [];
  for (let partNumber = 1; partNumber <= parts; partNumber += 1) {
    const slice = file.slice(
      (partNumber - 1) * MULTIPART_PART_SIZE,
      partNumber * MULTIPART_PART_SIZE,
    );
    let etag: string;
    if (session.streaming) {
      etag = await putMultipartPart(session.upload_id, partNumber, slice);
    } else {
      const url = session.part_urls?.[partNumber - 1];
      if (!url) throw new Error("the server did not hand out a URL for this part");
      etag = await presignedPut(url, slice);
    }
    uploaded.push({ part_number: partNumber, etag });
    onProgress?.({ done: partNumber, total: parts });
  }
  return completeMultipart(file, session, uploaded, ref);
}
