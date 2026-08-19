import { useEffect, useState, type FormEvent } from "react";
import { ArrowLeft, KeyRound, Link2, RadioTower, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { BrandMark } from "@/components/brand-mark";
import { api, post } from "./api";
import type { AccountBinding, Membership } from "./types";

interface Props {
  onBack: () => void;
  onOpenSpace: (spaceID: string) => void;
}

export default function AccountBindings({ onBack, onOpenSpace }: Props) {
  const [bindings, setBindings] = useState<AccountBinding[]>([]);
  const [selected, setSelected] = useState<AccountBinding | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api<{ bindings: AccountBinding[] }>("/api/account-bindings")
      .then((result) => {
        if (!cancelled) setBindings(Array.isArray(result.bindings) ? result.bindings : []);
      })
      .catch((caught) => {
        if (!cancelled) setError(caught instanceof Error ? caught.message : "读取频道账号失败");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  function updateMembership(spaceID: string, membership: Membership) {
    setBindings((current) => current.map((binding) => binding.space.id === spaceID
      ? { ...binding, space: { ...binding.space, is_bound: true }, membership }
      : binding));
    setSelected(null);
  }

  return (
    <div className="game-canvas flex min-h-svh flex-col">
      <header className="game-topbar flex h-16 shrink-0 items-center gap-3 px-3 sm:px-6">
        <Button variant="ghost" onClick={onBack}><ArrowLeft data-icon="inline-start" /><span className="hidden sm:inline">频道大厅</span></Button>
        <Separator orientation="vertical" className="h-8" />
        <BrandMark className="size-9 shrink-0" aria-hidden="true" />
        <div className="min-w-0"><h1 className="truncate text-sm font-semibold">频道账号</h1><p className="truncate text-xs text-muted-foreground">管理各频道的 New API 身份</p></div>
      </header>

      <main className="min-w-0 flex-1 overflow-auto">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8 lg:py-10">
          <section className="flex max-w-2xl flex-col gap-2" aria-labelledby="bindings-title">
            <Badge variant="outline" className="w-fit"><Link2 data-icon="inline-start" />频道账号</Badge>
            <h2 id="bindings-title" className="font-heading text-3xl font-semibold tracking-tight sm:text-4xl">一个频道，一个身份</h2>
            <p className="text-sm leading-6 text-muted-foreground">如果你在该频道连接的 New API 中已有账号，可使用该账号的 System Access Token 重新绑定。不同频道互不影响。</p>
          </section>

          {error && <Alert variant="destructive"><AlertTitle>无法读取频道账号</AlertTitle><AlertDescription>{error}</AlertDescription></Alert>}

          {loading ? (
            <div className="grid gap-4 sm:grid-cols-2">{Array.from({ length: 4 }, (_, index) => <BindingSkeleton key={index} />)}</div>
          ) : bindings.length === 0 ? (
            <Empty className="min-h-80 border bg-card"><EmptyHeader><EmptyMedia variant="icon"><RadioTower /></EmptyMedia><EmptyTitle>还没有可管理的频道账号</EmptyTitle><EmptyDescription>先创建频道或使用邀请码加入频道，这里就会显示对应账号。</EmptyDescription></EmptyHeader></Empty>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              {bindings.map((binding) => {
                const bound = !!binding.membership.newapi_user_id;
                const displayName = binding.membership.newapi_display_name || binding.membership.newapi_username;
                return (
                  <Card key={binding.space.id} className="h-full [--card-spacing:--spacing(6)]">
                    <CardHeader>
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="game-room-avatar grid size-11 shrink-0 place-items-center rounded-xl font-heading font-semibold">{(binding.space.name || "频").slice(0, 1).toUpperCase()}</span>
                        <div className="min-w-0"><CardTitle className="truncate text-lg">{binding.space.name}</CardTitle><CardDescription>{binding.space.can_manage ? "你管理的频道" : "已加入的频道"}</CardDescription></div>
                      </div>
                      <CardAction><Badge variant={bound ? "secondary" : "outline"}>{bound ? "已绑定" : "待绑定"}</Badge></CardAction>
                    </CardHeader>
                    <CardContent className="flex flex-1 flex-col gap-4">
                      <div className="rounded-xl border bg-muted/35 p-4">
                        <p className="text-xs font-medium text-muted-foreground">当前 New API 账号</p>
                        {bound ? (
                          <div className="mt-2 flex items-end justify-between gap-3"><div className="min-w-0"><p className="truncate font-heading text-lg font-semibold">{displayName}</p><p className="truncate text-sm text-muted-foreground">@{binding.membership.newapi_username}</p></div><span className="shrink-0 font-mono text-xs text-muted-foreground">Token ····{binding.membership.user_token_last4}</span></div>
                        ) : (
                          <p className="mt-2 text-sm text-muted-foreground">尚未绑定，可使用已有账号的 Token 完成绑定。</p>
                        )}
                      </div>
                      <div className="mt-auto grid grid-cols-2 gap-2">
                        <Button variant="outline" onClick={() => onOpenSpace(binding.space.id)}><RadioTower data-icon="inline-start" />进入频道</Button>
                        <Button onClick={() => setSelected(binding)}>{bound ? <RefreshCw data-icon="inline-start" /> : <KeyRound data-icon="inline-start" />}{bound ? "更换账号" : "绑定账号"}</Button>
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}
        </div>
      </main>

      <BindingDialog binding={selected} onClose={() => setSelected(null)} onBound={updateMembership} />
    </div>
  );
}

function BindingDialog({ binding, onClose, onBound }: {
  binding: AccountBinding | null;
  onClose: () => void;
  onBound: (spaceID: string, membership: Membership) => void;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setToken("");
    setError("");
  }, [binding?.space.id]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!binding) return;
    setBusy(true);
    setError("");
    try {
      const result = await post<{ membership: Membership }>(`/api/spaces/${binding.space.id}/bind`, { token });
      onBound(binding.space.id, result.membership);
      toast.success(`“${binding.space.name}”的账号已更新`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "绑定失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={binding !== null} onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>{binding?.membership.newapi_user_id ? "更换频道账号" : "绑定已有账号"}</DialogTitle><DialogDescription>为“{binding?.space.name}”绑定该频道 New API 中已有的账号。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <Alert><KeyRound /><AlertTitle>切换前请确认已离桌</AlertTitle><AlertDescription>如果你仍坐在该频道任意牌桌上，系统会拒绝切换，避免离桌结算进入错误账号。</AlertDescription></Alert>
            <Field><FieldLabel htmlFor="channel-account-token">System Access Token</FieldLabel><Input id="channel-account-token" name="channel-account-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="粘贴已有账号的 Token…" autoComplete="off" spellCheck={false} required /><FieldDescription>Token 只用于验证身份并加密保存，不会在页面中完整显示。</FieldDescription></Field>
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" disabled={busy} onClick={onClose}>取消</Button><Button disabled={busy || !token.trim()}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在验证…" : "验证并绑定"}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function BindingSkeleton() {
  return <Card aria-hidden="true"><CardHeader><div className="flex items-center gap-3"><Skeleton className="size-11 rounded-xl" /><div className="grid flex-1 gap-2"><Skeleton className="h-5 w-1/2" /><Skeleton className="h-3 w-1/3" /></div></div></CardHeader><CardContent className="grid gap-4"><Skeleton className="h-24 w-full rounded-xl" /><Skeleton className="h-10 w-full" /></CardContent></Card>;
}
