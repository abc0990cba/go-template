import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import { ExplainerBanner } from "@/components/explainer-banner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { settings as fetchSettings, updateSetting } from "./api";
import type { SettingView } from "./types";

export function AdminSettingsPage() {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ["settings"], queryFn: fetchSettings });

  const save = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => updateSetting(key, value),
    onSuccess: () => {
      toast.success("Setting saved");
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["public-settings"] });
    },
    onError: (error) => toast.error(error.message),
  });

  const groups = new Map<string, SettingView[]>();
  for (const setting of settings.data ?? []) {
    const list = groups.get(setting.group) ?? [];
    list.push(setting);
    groups.set(setting.group, list);
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Settings &amp; feature flags</h1>
        <p className="text-sm text-muted-foreground">
          Runtime configuration stored in the database — flip flags without a redeploy.
        </p>
      </div>
      <ExplainerBanner
        title="Settings — configuration as data"
        howTo={[
          "Boolean rows are feature flags the code reads through Settings.Bool; ints and strings are typed too.",
          "Public flags are exposed at /api/settings/public so the login page can react before authentication.",
          "Every change is audited with the previous value.",
        ]}
      >
        The catalog is code-defined (<code>service.SettingDefinitions</code>) so types and defaults
        ship with the binary; only values live in the database. Add a definition, and the admin UI
        picks it up automatically.
      </ExplainerBanner>

      {settings.isPending && <Loader2 className="size-5 animate-spin" />}
      {[...groups.entries()].map(([group, rows]) => (
        <Card key={group}>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">{group}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {rows.map((setting) => (
              <SettingRow
                key={setting.key}
                setting={setting}
                canEdit={save.isPending !== undefined}
                onSave={(value) => save.mutate({ key: setting.key, value })}
                pending={save.isPending}
              />
            ))}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function SettingRow({
  setting,
  onSave,
  pending,
}: {
  setting: SettingView;
  canEdit?: boolean;
  onSave: (value: string) => void;
  pending: boolean;
}) {
  // The parent keys rows by server value, so a refetch with a new value
  // remounts this row with fresh local state — no sync effect needed.
  const [value, setValue] = useState(setting.value);

  if (setting.is_flag) {
    return (
      <div className="flex items-center justify-between gap-3 border-b pb-3 last:border-b-0">
        <div>
          <p className="text-sm font-medium">{setting.label}</p>
          <code className="text-xs text-muted-foreground">{setting.key}</code>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline">{value === "true" ? "on" : "off"}</Badge>
          <Switch
            checked={value === "true"}
            onCheckedChange={(checked) => {
              setValue(String(checked));
              onSave(String(checked));
            }}
            disabled={pending}
            aria-label={setting.label}
          />
        </div>
      </div>
    );
  }
  return (
    <div className="flex items-center justify-between gap-3 border-b pb-3 last:border-b-0">
      <div>
        <p className="text-sm font-medium">{setting.label}</p>
        <code className="text-xs text-muted-foreground">{setting.key}</code>
      </div>
      <div className="flex items-center gap-2">
        <Input
          className="w-28"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          aria-label={setting.label}
        />
        <Button
          size="sm"
          variant="outline"
          onClick={() => onSave(value)}
          disabled={pending || value === setting.value}
        >
          Save
        </Button>
      </div>
    </div>
  );
}
