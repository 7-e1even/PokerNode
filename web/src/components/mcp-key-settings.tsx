import { useEffect, useState } from "react";
import { Copy, KeyRound, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, post, put, remove } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";

interface MCPKeyStatus {
  exists: boolean;
  last4?: string;
  created_at?: string;
}

export function MCPKeySettings() {
  const [status, setStatus] = useState<MCPKeyStatus | null>(null);
  const [generatedKey, setGeneratedKey] = useState("");
  const [agentControlEnabled, setAgentControlEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmAction, setConfirmAction] = useState<"rotate" | "revoke" | null>(null);
  const endpoint = `${window.location.origin}/mcp`;

  useEffect(() => {
    let cancelled = false;
    void Promise.all([
      api<{ status: MCPKeyStatus }>("/api/me/mcp-key"),
      api<{ enabled: boolean }>("/api/me/agent-control"),
    ])
      .then(([keyResult, controlResult]) => {
        if (!cancelled) {
          setStatus(keyResult.status);
          setAgentControlEnabled(controlResult.enabled);
        }
      })
      .catch((caught) => { if (!cancelled) setError(caught instanceof Error ? caught.message : "读取 MCP 设置失败"); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, []);

  async function generate() {
    setBusy(true);
    setError("");
    try {
      const result = await post<{ mcp_key: string; status: MCPKeyStatus }>("/api/me/mcp-key");
      setStatus(result.status);
      setGeneratedKey(result.mcp_key);
      toast.success(status?.exists ? "MCP Key 已轮换" : "MCP Key 已生成");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "生成 MCP Key 失败");
    } finally {
      setBusy(false);
    }
  }

  async function revoke() {
    setBusy(true);
    setError("");
    try {
      await remove("/api/me/mcp-key");
      setStatus({ exists: false });
      setAgentControlEnabled(false);
      setGeneratedKey("");
      toast.success("MCP Key 已撤销");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "撤销 MCP Key 失败");
    } finally {
      setBusy(false);
    }
  }

  async function updateAgentControl(enabled: boolean) {
    setBusy(true);
    setError("");
    try {
      const result = await put<{ enabled: boolean }>("/api/me/agent-control", { enabled });
      setAgentControlEnabled(result.enabled);
      toast.success(result.enabled ? "已交给 Agent 托管" : "已收回牌局控制");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "切换托管状态失败");
    } finally {
      setBusy(false);
    }
  }

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label}已复制`);
    } catch {
      toast.error(`${label}复制失败，请手动复制`);
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Agent MCP</CardTitle>
          <CardDescription>管理 Agent 连接地址、个人 Key 和牌局托管权限。</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
          {loading ? (
            <div className="flex min-h-40 items-center justify-center"><Spinner /></div>
          ) : (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="mcp-endpoint">MCP 地址</FieldLabel>
                <InputGroup>
                  <InputGroupInput id="mcp-endpoint" value={endpoint} readOnly />
                  <InputGroupAddon align="inline-end">
                    <InputGroupButton size="icon-xs" onClick={() => void copy(endpoint, "MCP 地址")} aria-label="复制 MCP 地址"><Copy /></InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
              </Field>
              <Field orientation="responsive">
                <FieldContent>
                  <FieldTitle>当前 Key</FieldTitle>
                  <FieldDescription>{status?.exists ? `创建于 ${formatDate(status.created_at || "")}` : "每个用户同时只有一把有效 Key。"}</FieldDescription>
                </FieldContent>
                <Badge variant={status?.exists ? "secondary" : "outline"}>{status?.exists ? `已启用 · 尾号 ${status.last4}` : "尚未生成"}</Badge>
              </Field>
              <Field orientation="responsive">
                <FieldContent>
                  <FieldTitle>Agent 托管</FieldTitle>
                  <FieldDescription>{agentControlEnabled ? "Agent 可操作牌局，网页操作已锁定。" : "当前由你操作，MCP 只能读取牌局。"}</FieldDescription>
                </FieldContent>
                <Switch checked={agentControlEnabled} disabled={busy || !status?.exists} onCheckedChange={(enabled) => void updateAgentControl(enabled)} aria-label="切换 Agent 托管" />
              </Field>
              {generatedKey && (
                <Alert>
                  <KeyRound />
                  <AlertDescription className="flex flex-col gap-3">
                    <p>请立即复制这把 Key。离开本页后，服务器不会再次显示完整内容。</p>
                    <InputGroup>
                      <InputGroupInput value={generatedKey} readOnly aria-label="新生成的 MCP Key" />
                      <InputGroupAddon align="inline-end"><InputGroupButton size="icon-xs" onClick={() => void copy(generatedKey, "MCP Key")} aria-label="复制 MCP Key"><Copy /></InputGroupButton></InputGroupAddon>
                    </InputGroup>
                  </AlertDescription>
                </Alert>
              )}
            </FieldGroup>
          )}
        </CardContent>
        <CardFooter className="justify-between gap-2">
          <div>{status?.exists && <Button type="button" variant="destructive" onClick={() => setConfirmAction("revoke")} disabled={busy || loading}><Trash2 data-icon="inline-start" />撤销 Key</Button>}</div>
          <Button type="button" onClick={() => status?.exists ? setConfirmAction("rotate") : void generate()} disabled={busy || loading}>
            {busy ? <Spinner data-icon="inline-start" /> : status?.exists ? <RefreshCw data-icon="inline-start" /> : <KeyRound data-icon="inline-start" />}
            {busy ? "处理中" : status?.exists ? "轮换 Key" : "生成 Key"}
          </Button>
        </CardFooter>
      </Card>
      <AlertDialog open={confirmAction !== null} onOpenChange={(nextOpen) => { if (!nextOpen && !busy) setConfirmAction(null); }}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>{confirmAction === "revoke" ? "撤销 MCP Key？" : "轮换 MCP Key？"}</AlertDialogTitle>
            <AlertDialogDescription>{confirmAction === "revoke" ? "使用当前 Key 的 Agent 会立即失去授权。之后可以重新生成。" : "旧 Key 会立即失效，所有 Agent 都需要改用新 Key。"}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={busy} onClick={(event) => {
              event.preventDefault();
              void (confirmAction === "revoke" ? revoke() : generate()).then(() => setConfirmAction(null));
            }}>
              {busy && <Spinner data-icon="inline-start" />}{busy ? "处理中" : confirmAction === "revoke" ? "确认撤销" : "确认轮换"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  return !value || Number.isNaN(date.getTime()) ? "未知时间" : date.toLocaleString();
}
