import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, Loader2, Plus, X } from "lucide-react";
import { ExplainerBanner } from "@/shared/components/explainer-banner";
import { Badge } from "@/shared/components/ui/badge";
import { Button } from "@/shared/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/components/ui/card";
import { Input } from "@/shared/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/components/ui/select";
import { addPolicy, listPolicies, removePolicy } from "./api";
import { useMe } from "@/shared/hooks/use-permissions";
import type { PolicyRule } from "./types";

export function AdminPoliciesPage() {
  const me = useMe();
  const queryClient = useQueryClient();
  const [ptype, setPtype] = useState<"p" | "g">("p");
  const policies = useQuery({
    queryKey: ["policies", ptype],
    queryFn: () => listPolicies(ptype),
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["policies"] });
    void queryClient.invalidateQueries({ queryKey: ["me"] });
  };

  const remove = useMutation({
    mutationFn: (rule: PolicyRule) => removePolicy(rule),
    onSuccess: () => {
      toast.success("Rule removed");
      invalidate();
    },
    onError: (error) => toast.error(error.message),
  });

  const canDelete = me.permissions.includes("policies:delete");

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Raw policies</h1>
        <p className="text-sm text-muted-foreground">
          Every rule the policy engine holds — the deepest level of control.
        </p>
      </div>
      <ExplainerBanner
        title="Policies — the raw editor"
        howTo={[
          'ptype "p" rows are permission rules: subject, domain, object, action.',
          'ptype "g" rows are role assignments: user, role, domain.',
          "Changes apply instantly; there is no draft state, so treat this page as a power tool.",
        ]}
      >
        <span className="inline-flex items-center gap-2">
          <AlertTriangle className="size-4 text-amber-500" />
          Use the Roles matrix for day-to-day editing. This page exists for edge cases the UI does
          not cover — direct user grants, wildcard subjects, debugging.
        </span>
      </ExplainerBanner>

      <div className="flex items-center gap-2">
        <Select value={ptype} onValueChange={(v) => setPtype(v as "p" | "g")}>
          <SelectTrigger className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="p">p — permissions</SelectItem>
            <SelectItem value="g">g — assignments</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardHeader className="py-3">
          <CardTitle className="text-sm">{policies.data?.length ?? 0} rules</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1 font-mono text-xs">
          {policies.isPending && <Loader2 className="size-4 animate-spin" />}
          {(policies.data ?? []).map((rule) => (
            <RuleRow
              key={ruleKey(rule)}
              rule={rule}
              canDelete={canDelete && !remove.isPending}
              onRemove={() => remove.mutate(rule)}
            />
          ))}
          {!policies.isPending && (policies.data ?? []).length === 0 && (
            <p className="font-sans text-sm text-muted-foreground">No rules of this type.</p>
          )}
        </CardContent>
      </Card>

      <AddRuleCard ptype={ptype} defaultDomain={me.active_org || "*"} onAdded={invalidate} />
    </div>
  );
}

function ruleKey(rule: PolicyRule): string {
  return [rule.ptype, rule.subject, rule.domain, rule.object ?? "", rule.action ?? ""].join("|");
}

function RuleRow({
  rule,
  onRemove,
  canDelete,
}: {
  rule: PolicyRule;
  onRemove: () => void;
  canDelete: boolean;
}) {
  return (
    <div className="flex items-center gap-2 rounded border px-2 py-1.5">
      <Badge variant="outline">{rule.ptype}</Badge>
      <span>{rule.subject}</span>
      <span className="text-muted-foreground">@ {rule.domain}</span>
      {rule.ptype === "p" && (
        <span>
          : {rule.object} · {rule.action}
        </span>
      )}
      {rule.ptype === "g" && <span>→ {rule.object}</span>}
      {canDelete && (
        <Button
          variant="ghost"
          size="icon"
          className="ml-auto size-6 text-destructive"
          onClick={onRemove}
          aria-label="Remove rule"
        >
          <X />
        </Button>
      )}
    </div>
  );
}

function AddRuleCard({
  ptype,
  defaultDomain,
  onAdded,
}: {
  ptype: "p" | "g";
  defaultDomain: string;
  onAdded: () => void;
}) {
  const [subject, setSubject] = useState("");
  const [domain, setDomain] = useState(defaultDomain);
  const [object, setObject] = useState("");
  const [action, setAction] = useState("");

  const add = useMutation({
    mutationFn: () =>
      addPolicy({
        ptype,
        subject: subject.trim(),
        domain: domain.trim() || "*",
        object: object.trim(),
        action: action.trim(),
      }),
    onSuccess: () => {
      toast.success("Rule added");
      setSubject("");
      setObject("");
      setAction("");
      onAdded();
    },
    onError: (error) => toast.error(error.message),
  });

  const incomplete =
    !subject.trim() || (ptype === "p" ? !object.trim() || !action.trim() : !object.trim());

  return (
    <Card>
      <CardHeader className="py-3">
        <CardTitle className="text-sm">Add rule</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-2">
        <Input
          className="w-48"
          placeholder={ptype === "p" ? "subject (role or user)" : "user id"}
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
        />
        {ptype === "p" ? (
          <>
            <Input
              className="w-32"
              placeholder="object"
              value={object}
              onChange={(e) => setObject(e.target.value)}
            />
            <Input
              className="w-32"
              placeholder="action"
              value={action}
              onChange={(e) => setAction(e.target.value)}
            />
          </>
        ) : (
          <Input
            className="w-48"
            placeholder="role key"
            value={object}
            onChange={(e) => setObject(e.target.value)}
          />
        )}
        <Input
          className="w-44"
          placeholder='domain ("*" = platform)'
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
        />
        <Button size="sm" onClick={() => add.mutate()} disabled={incomplete || add.isPending}>
          <Plus /> Add
        </Button>
      </CardContent>
    </Card>
  );
}
