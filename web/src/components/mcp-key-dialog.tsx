import { useEffect, useState } from "react";
import { Copy, KeyRound, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, post, put, remove } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";

interface MCPKeyStatus {
  exists: boolean;
  last4?: string;
  created_at?: string;
}

export function MCPKeyDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [status, setStatus] = useState<MCPKeyStatus | null>(null);
  const [generatedKey, setGeneratedKey] = useState("");
  const [agentControlEnabled, setAgentControlEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmAction, setConfirmAction] = useState<"rotate" | "revoke" | null>(null);
  const endpoint = `${window.location.origin}/mcp`;

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setGeneratedKey("");
    setStatus(null);
    setAgentControlEnabled(false);
    setError("");
    setLoading(true);
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
      .catch((caught) => { if (!cancelled) setError(caught instanceof Error ? caught.message : "读取 MCP Key 状态失败"); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [open]);

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

  return (<>
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!busy) onOpenChange(nextOpen); }}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Agent MCP</DialogTitle>
          <DialogDescription>每个用户只有一把独立 Key。Agent 使用它连接 HTTP MCP，并以你的玩家身份行动。</DialogDescription>
        </DialogHeader>

        {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
        {loading ? (
          <div className="flex min-h-32 items-center justify-center"><Spinner /></div>
        ) : (
          <div className="flex flex-col gap-4">
            <Field>
              <FieldLabel htmlFor="mcp-endpoint">MCP 地址</FieldLabel>
              <InputGroup>
                <InputGroupInput id="mcp-endpoint" value={endpoint} readOnly />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton size="icon-xs" onClick={() => void copy(endpoint, "MCP 地址")} aria-label="复制 MCP 地址"><Copy /></InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </Field>

            <Card>
              <CardHeader>
                <CardTitle>当前 Key</CardTitle>
                <CardDescription>{status?.exists ? `已启用 · 尾号 ${status.last4}` : "尚未生成"}</CardDescription>
                {status?.created_at && <CardAction><time>{formatDate(status.created_at)}</time></CardAction>}
              </CardHeader>
            </Card>

            <div className="flex items-center justify-between gap-4 rounded-xl border p-4">
              <div className="min-w-0">
                <p className="text-sm font-medium">Agent 托管</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {agentControlEnabled ? "Agent 可操作牌局，网页操作已锁定。" : "当前由你操作，MCP 只能读取牌局。"}
                </p>
              </div>
              <Switch
                checked={agentControlEnabled}
                disabled={busy || !status?.exists}
                onCheckedChange={(enabled) => void updateAgentControl(enabled)}
                aria-label="切换 Agent 托管"
              />
            </div>

            {generatedKey && (
              <Alert>
                <KeyRound />
                <AlertDescription className="flex flex-col gap-3">
                  <p>请立即复制这把 Key。关闭窗口后，服务器不会再次显示完整内容。</p>
                  <InputGroup>
                    <InputGroupInput value={generatedKey} readOnly aria-label="新生成的 MCP Key" />
                    <InputGroupAddon align="inline-end">
                      <InputGroupButton size="icon-xs" onClick={() => void copy(generatedKey, "MCP Key")} aria-label="复制 MCP Key"><Copy /></InputGroupButton>
                    </InputGroupAddon>
                  </InputGroup>
                </AlertDescription>
              </Alert>
            )}
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-between">
          <div>{status?.exists && <Button type="button" variant="destructive" onClick={() => setConfirmAction("revoke")} disabled={busy || loading}><Trash2 data-icon="inline-start" />撤销 Key</Button>}</div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>关闭</Button>
            <Button type="button" onClick={() => status?.exists ? setConfirmAction("rotate") : void generate()} disabled={busy || loading}>
              {busy ? <Spinner data-icon="inline-start" /> : status?.exists ? <RefreshCw data-icon="inline-start" /> : <KeyRound data-icon="inline-start" />}
              {busy ? "处理中" : status?.exists ? "轮换 Key" : "生成 Key"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    <AlertDialog open={confirmAction !== null} onOpenChange={(nextOpen) => { if (!nextOpen && !busy) setConfirmAction(null); }}>
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>{confirmAction === "revoke" ? "撤销 MCP Key？" : "轮换 MCP Key？"}</AlertDialogTitle>
          <AlertDialogDescription>
            {confirmAction === "revoke"
              ? "使用当前 Key 的 Agent 会立即失去授权。之后可以重新生成。"
              : "旧 Key 会立即失效，所有 Agent 都需要改用新 Key。"}
          </AlertDialogDescription>
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
  </>);
}

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
