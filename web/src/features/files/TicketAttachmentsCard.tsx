import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { toast } from "sonner";
import { Loader2, Paperclip, Upload } from "lucide-react";

import { Button } from "@/shared/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/shared/components/ui/card";
import { ApiError } from "@/shared/api/core";

import { listTicketAttachments, uploadAttachment } from "./api";
import type { AttachmentRef, StoredFile } from "./api";

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

/**
 * TicketAttachmentsCard lists a ticket's attachments and uploads new ones.
 * The upload transport picks itself per file (single-shot vs resumable
 * session); callers only see the stored metadata. Downloads are plain
 * same-origin links — the session cookie authorizes them.
 */
export function TicketAttachmentsCard({ ticketId }: { ticketId: string }) {
  const queryClient = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);

  const attachments = useQuery({
    queryKey: ["tickets", "attachments", ticketId],
    queryFn: () => listTicketAttachments(ticketId),
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["tickets", "attachments", ticketId] });
  };

  const upload = useMutation({
    mutationFn: ({ file, ref }: { file: globalThis.File; ref: AttachmentRef }) =>
      uploadAttachment(file, ref, setProgress),
    onSuccess: (stored: StoredFile) => {
      toast.success(`Attached ${stored.filename} (${formatSize(stored.size_bytes)})`);
      invalidate();
    },
    onError: (error) => {
      const detail = error instanceof ApiError ? error.message : "upload failed";
      toast.error(detail);
    },
    onSettled: () => setProgress(null),
  });

  const onPicked = (files: FileList | null) => {
    const file = files?.[0];
    if (!file) return;
    upload.mutate({ file, ref: { type: "ticket", id: ticketId } });
    if (inputRef.current) inputRef.current.value = "";
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Paperclip className="size-4" /> Attachments
        </CardTitle>
        <CardDescription>Stored in the file store; large files upload in parts.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {attachments.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (attachments.data ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">No attachments yet.</p>
        ) : (
          <ul className="divide-y text-sm">
            {(attachments.data ?? []).map((file) => (
              <li key={file.id} className="flex items-center justify-between py-1.5">
                <a
                  className="underline decoration-dotted underline-offset-4 hover:underline"
                  href={`/api/files/${file.id}/content`}
                >
                  {file.filename}
                </a>
                <span className="text-muted-foreground">{formatSize(file.size_bytes)}</span>
              </li>
            ))}
          </ul>
        )}

        <input
          ref={inputRef}
          type="file"
          className="hidden"
          onChange={(event) => onPicked(event.target.files)}
        />
        <Button
          size="sm"
          variant="outline"
          onClick={() => inputRef.current?.click()}
          disabled={upload.isPending}
        >
          {upload.isPending ? (
            <>
              <Loader2 className="size-4 animate-spin" />
              {progress ? `Uploading ${progress.done}/${progress.total}…` : "Uploading…"}
            </>
          ) : (
            <>
              <Upload className="size-4" /> Attach a file
            </>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}
