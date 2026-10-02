import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { toast } from "sonner";
import { useState } from "react";
import { Loader2, ShieldCheck, ShieldOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { mfaDisable, mfaEnroll, mfaStatus, mfaVerify } from "./api";

export function MFACard() {
  const queryClient = useQueryClient();
  const status = useQuery({ queryKey: ["mfa", "status"], queryFn: mfaStatus });

  // Enrollment state machine: idle → qr (secret shown) → recovery codes.
  const [enrollment, setEnrollment] = useState<{ secret: string; otpauth_url: string } | null>(
    null,
  );
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [disablePassword, setDisablePassword] = useState("");
  const [disableCode, setDisableCode] = useState("");

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["mfa"] });

  const enroll = useMutation({
    mutationFn: mfaEnroll,
    onSuccess: (result) => {
      setCode("");
      setQrDataUrl(null);
      // The provisioning URI renders as a QR; the raw secret stays as the
      // manual-entry fallback.
      void QRCode.toDataURL(result.otpauth_url, { width: 192 })
        .then(setQrDataUrl)
        .catch(() => setQrDataUrl(null));
      setEnrollment(result);
    },
    onError: (error) => toast.error(error.message),
  });

  const verify = useMutation({
    mutationFn: () => mfaVerify(code.trim()),
    onSuccess: ({ recovery_codes }) => {
      setEnrollment(null);
      setCode("");
      setRecoveryCodes(recovery_codes);
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const disable = useMutation({
    mutationFn: () => mfaDisable(disablePassword, disableCode.trim()),
    onSuccess: () => {
      toast.success("Two-factor authentication disabled");
      setDisablePassword("");
      setDisableCode("");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const enabled = status.data?.enabled ?? false;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="size-4" /> Two-factor authentication
        </CardTitle>
        <CardDescription>
          {enabled
            ? "Enabled — sign-in asks for a code from your authenticator app."
            : status.data?.required
              ? "Required by policy for your account — set it up to be able to sign in."
              : "Add a time-based one-time password (TOTP) as a second sign-in step."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {recoveryCodes && (
          <div className="rounded border border-emerald-500/40 bg-emerald-500/10 p-2 text-xs">
            <p className="font-medium">
              Save your recovery codes now — each works once and they are shown only this once:
            </p>
            <div className="mt-1 grid grid-cols-2 gap-1 font-mono sm:grid-cols-5">
              {recoveryCodes.map((recovery) => (
                <code key={recovery}>{recovery}</code>
              ))}
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              onClick={() => setRecoveryCodes(null)}
            >
              I saved them
            </Button>
          </div>
        )}

        {enabled ? (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              To turn two-factor off, confirm your password and a current code.
            </p>
            <div className="flex flex-wrap gap-2">
              <Input
                className="w-48"
                type="password"
                autoComplete="current-password"
                placeholder="Password"
                value={disablePassword}
                onChange={(event) => setDisablePassword(event.target.value)}
              />
              <Input
                className="w-40"
                placeholder="Current code"
                value={disableCode}
                onChange={(event) => setDisableCode(event.target.value)}
              />
              <Button
                size="sm"
                variant="destructive"
                disabled={!disablePassword || !disableCode || disable.isPending}
                onClick={() => disable.mutate()}
              >
                {disable.isPending ? <Loader2 className="size-4 animate-spin" /> : <ShieldOff />}
                Disable
              </Button>
            </div>
          </div>
        ) : enrollment ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-start gap-4">
              {qrDataUrl && (
                <img
                  src={qrDataUrl}
                  alt="TOTP provisioning QR code"
                  className="size-40 rounded border bg-white p-1"
                />
              )}
              <div className="min-w-0 space-y-1 text-sm">
                <p>Scan with your authenticator app, or enter the secret manually:</p>
                <code className="block break-all rounded bg-muted px-2 py-1 font-mono text-xs">
                  {enrollment.secret}
                </code>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Input
                className="w-40"
                placeholder="6-digit code"
                value={code}
                onChange={(event) => setCode(event.target.value)}
              />
              <Button
                size="sm"
                disabled={!code.trim() || verify.isPending}
                onClick={() => verify.mutate()}
              >
                {verify.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                Confirm
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setEnrollment(null);
                  setQrDataUrl(null);
                }}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <Button size="sm" onClick={() => enroll.mutate()} disabled={enroll.isPending}>
            {enroll.isPending ? <Loader2 className="size-4 animate-spin" /> : <ShieldCheck />}
            Set up two-factor
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
