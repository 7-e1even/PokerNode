import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { ArrowLeft, CircleDollarSign, Minus, Plus, RadioTower, RefreshCw, Search, UsersRound } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { api, post } from "./api";
import type { ManagedBalanceMember, ManagedBalancesResponse, WalletOperation } from "./types";

type BalanceSpace = {
  id: string;
  name: string;
  member_count?: number;
  bound_member_count?: number;
  newapi_base_url?: string;
};
type Direction = "add" | "subtract";

export default function BalanceManager({ spaces, initialSpaceID, onBack }: {
  spaces: BalanceSpace[];
  initialSpaceID?: string;
  onBack?: () => void;
}) {
  const [spaceID, setSpaceID] = useState(initialSpaceID || spaces[0]?.id || "");
  const [data, setData] = useState<ManagedBalancesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<ManagedBalanceMember | null>(null);
  const [query, setQuery] = useState("");
  const loadSequence = useRef(0);

  useEffect(() => {
    if (spaces.some((space) => space.id === spaceID)) return;
    setSpaceID(initialSpaceID || spaces[0]?.id || "");
  }, [initialSpaceID, spaceID, spaces]);

  async function load(showLoading = true) {
    const sequence = ++loadSequence.current;
    if (!spaceID) {
      setData(null);
      setLoading(false);
      return;
    }
    if (showLoading) setLoading(true);
    setError("");
    try {
      const result = await api<ManagedBalancesResponse>(`/api/spaces/${spaceID}/managed-balances`);
      if (sequence === loadSequence.current) setData(result);
    } catch (caught) {
      if (sequence === loadSequence.current) setError(caught instanceof Error ? caught.message : "读取成员余额失败");
    } finally {
      if (showLoading && sequence === loadSequence.current) setLoading(false);
    }
  }

  useEffect(() => {
    setData(null);
    setEditing(null);
    void load();
  }, [spaceID]);

  const selectedSpace = spaces.find((space) => space.id === spaceID);
  const visibleSpaces = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return spaces;
    return spaces.filter((space) => `${space.name} ${space.newapi_base_url || ""}`.toLowerCase().includes(needle));
  }, [query, spaces]);
  const showChannelDirectory = !onBack;

  const content = (
    <div className="flex flex-1 flex-col gap-5 overflow-auto bg-muted/20 p-4 sm:p-6 lg:p-8">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="font-heading text-xl font-semibold">按频道管理余额</h2>
          <p className="mt-1 text-sm text-muted-foreground">每个频道独立使用自己的 New API 节点和成员余额，调整记录不会混在其他频道。</p>
        </div>
        <Badge variant="outline">{showChannelDirectory ? `${spaces.length} 个可管理频道` : "当前频道"}</Badge>
      </div>

      {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

      <div className={cn("grid min-h-0 flex-1 gap-5", showChannelDirectory && "xl:grid-cols-[18rem_minmax(0,1fr)]")}>
        {showChannelDirectory && (
          <Card className="min-h-0">
            <CardHeader><CardTitle>可管理频道</CardTitle><CardDescription>选择一个频道后再调整成员余额。</CardDescription><CardAction><Badge variant="secondary">{spaces.length}</Badge></CardAction></CardHeader>
            <CardContent className="flex min-h-0 flex-col gap-3">
              <InputGroup><InputGroupInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索频道或节点" aria-label="搜索余额频道" /><InputGroupAddon><Search /></InputGroupAddon></InputGroup>
              <ScrollArea className="max-h-[calc(100svh-21rem)] min-h-48">
                <div className="flex flex-col gap-1 pr-3">
                  {visibleSpaces.length === 0 ? <p className="p-6 text-center text-sm text-muted-foreground">没有匹配的频道</p> : visibleSpaces.map((space) => (
                    <Button key={space.id} className="h-auto justify-start px-3 py-2.5 text-left" variant={space.id === spaceID ? "secondary" : "ghost"} onClick={() => setSpaceID(space.id)}>
                      <RadioTower data-icon="inline-start" />
                      <span className="min-w-0 flex-1"><strong className="block truncate text-sm">{space.name}</strong><small className="block truncate text-xs text-muted-foreground">{space.member_count === undefined ? "独立频道余额" : `${space.bound_member_count || 0}/${space.member_count} 名成员已绑定`}</small></span>
                    </Button>
                  ))}
                </div>
              </ScrollArea>
            </CardContent>
          </Card>
        )}

        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>{data?.space.name || selectedSpace?.name || "频道余额"}</CardTitle>
            <CardDescription>{data ? `${data.members.length} 名成员 · 仅显示本频道余额 · 金额按美元展示` : spaceID ? "正在读取当前频道成员与实时余额" : "请选择左侧频道"}</CardDescription>
            <CardAction><Button size="sm" variant="outline" disabled={loading || !spaceID} onClick={() => void load()}><RefreshCw data-icon="inline-start" className={loading ? "animate-spin" : ""} />刷新当前频道</Button></CardAction>
          </CardHeader>
          <CardContent>
            {!spaceID ? (
              <Empty className="min-h-64"><EmptyHeader><EmptyMedia variant="icon"><RadioTower /></EmptyMedia><EmptyTitle>没有可管理的频道</EmptyTitle><EmptyDescription>为账号分配频道范围后，会在这里按频道显示。</EmptyDescription></EmptyHeader></Empty>
            ) : loading ? <BalanceTableSkeleton /> : !data || data.members.length === 0 ? (
              <Empty className="min-h-64"><EmptyHeader><EmptyMedia variant="icon"><UsersRound /></EmptyMedia><EmptyTitle>当前频道还没有成员</EmptyTitle><EmptyDescription>成员加入这个频道后会显示在这里。</EmptyDescription></EmptyHeader></Empty>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader><TableRow><TableHead>本频道成员</TableHead><TableHead>New API 账号</TableHead><TableHead className="text-right">当前频道余额</TableHead><TableHead className="w-28 text-right">操作</TableHead></TableRow></TableHeader>
                  <TableBody>{data.members.map((member) => (
                    <TableRow key={member.user_id}>
                      <TableCell><div className="flex items-center gap-3"><Avatar><AvatarFallback>{initials(member.poker_display_name)}</AvatarFallback></Avatar><strong>{member.poker_display_name}</strong></div></TableCell>
                      <TableCell>{member.bound ? <span><span className="block text-sm">{member.newapi_display_name || member.newapi_username}</span><small className="text-muted-foreground">@{member.newapi_username}</small></span> : <Badge variant="outline">本频道待绑定</Badge>}</TableCell>
                      <TableCell className="text-right tabular-nums">{member.error ? <span className="text-sm text-destructive">{member.error}</span> : member.balance ? <strong>{formatUSD(member.balance.cents)}</strong> : <span className="text-muted-foreground">—</span>}</TableCell>
                      <TableCell className="text-right"><Button size="sm" variant="outline" disabled={!member.bound || !member.balance || Boolean(member.error)} onClick={() => setEditing(member)}>调整</Button></TableCell>
                    </TableRow>
                  ))}</TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {editing && data && <AdjustBalanceDialog spaceID={spaceID} spaceName={data.space.name || selectedSpace?.name || "当前频道"} maxAdjustmentCents={data.space.max_adjustment_cents} member={editing} onClose={() => setEditing(null)} onAdjusted={(member) => {
        setData((current) => current ? { ...current, members: current.members.map((item) => item.user_id === member.user_id ? member : item) } : current);
        setEditing(null);
      }} />}
    </div>
  );

  if (!onBack) return content;
  return (
    <div className="flex min-h-svh flex-col bg-background">
      <header className="flex h-16 shrink-0 items-center gap-3 border-b px-4 sm:px-6">
        <Button size="icon" variant="ghost" onClick={onBack} aria-label="返回频道"><ArrowLeft /></Button>
        <BrandMark className="size-9" />
        <div><h1 className="text-sm font-semibold">余额管理</h1><p className="text-xs text-muted-foreground">仅限本频道成员</p></div>
      </header>
      {content}
    </div>
  );
}

function AdjustBalanceDialog({ spaceID, spaceName, maxAdjustmentCents, member, onClose, onAdjusted }: {
  spaceID: string;
  spaceName: string;
  maxAdjustmentCents: number;
  member: ManagedBalanceMember;
  onClose: () => void;
  onAdjusted: (member: ManagedBalanceMember) => void;
}) {
  const [direction, setDirection] = useState<Direction>("add");
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const cents = useMemo(() => dollarsToCents(amount), [amount]);
  const maximumCents = direction === "subtract"
    ? Math.min(maxAdjustmentCents, member.balance?.cents || 0)
    : maxAdjustmentCents;
  const amountError = amount.trim() && !cents
    ? "请输入大于 0 且最多两位小数的金额"
    : cents > maximumCents
      ? direction === "subtract"
        ? `扣减金额不能超过当前余额 ${formatUSD(maximumCents)}`
        : `调整金额过大，单次最多为 ${formatUSD(maximumCents)}`
      : "";

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!cents || amountError) {
      setError(amountError || "请输入大于 0 且最多两位小数的金额");
      return;
    }
    if (reason.trim().length < 2) {
      setError("请填写至少 2 个字符的调整原因");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await post<{ member: ManagedBalanceMember; operation: WalletOperation }>(`/api/spaces/${spaceID}/managed-balances/${member.user_id}/adjust`, {
        direction, amount_cents: cents, reason: reason.trim(),
      });
      onAdjusted(result.member);
      toast.success(`${member.poker_display_name} 的余额已${direction === "add" ? "增加" : "扣减"} ${formatUSD(cents)}`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "余额调整失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>调整 {member.poker_display_name} 的余额</DialogTitle><DialogDescription>频道：{spaceName} · 当前余额 {member.balance ? formatUSD(member.balance.cents) : "未知"}。提交后只会同步到这个频道的 New API。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <Field><FieldLabel>调整类型</FieldLabel><ToggleGroup type="single" variant="outline" spacing={0} value={direction} onValueChange={(value) => value && setDirection(value as Direction)} className="w-full"><ToggleGroupItem value="add" className="flex-1"><Plus />增加</ToggleGroupItem><ToggleGroupItem value="subtract" className="flex-1"><Minus />扣减</ToggleGroupItem></ToggleGroup></Field>
            <Field data-invalid={Boolean(amountError)}><FieldLabel htmlFor="balance-amount">金额（美元）</FieldLabel><Input id="balance-amount" inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} placeholder="0.00" autoFocus required aria-invalid={Boolean(amountError)} />{amountError ? <FieldError>{amountError}</FieldError> : <FieldDescription>{cents ? `本次${direction === "add" ? "增加" : "扣减"} ${formatUSD(cents)} · ` : ""}{direction === "add" ? `单次最多 ${formatUSD(maximumCents)}` : `最多可扣减 ${formatUSD(maximumCents)}（当前余额）`}</FieldDescription>}</Field>
            <Field><FieldLabel htmlFor="balance-reason">调整原因</FieldLabel><Textarea id="balance-reason" value={reason} onChange={(event) => setReason(event.target.value)} placeholder="例如：活动奖励、人工退款" minLength={2} maxLength={200} required /></Field>
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" disabled={busy} onClick={onClose}>取消</Button><Button disabled={busy || !cents || Boolean(amountError) || reason.trim().length < 2}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在同步…" : `确认${direction === "add" ? "增加" : "扣减"}`}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function BalanceTableSkeleton() {
  return <div className="space-y-3">{Array.from({ length: 4 }, (_, index) => <div key={index} className="flex items-center gap-3 py-2"><Skeleton className="size-10 rounded-full" /><Skeleton className="h-4 w-36" /><Skeleton className="ml-auto h-4 w-20" /><Skeleton className="h-8 w-16" /></div>)}</div>;
}

function dollarsToCents(value: string) {
  const normalized = value.trim();
  if (!/^\d+(?:\.\d{1,2})?$/.test(normalized)) return 0;
  const [whole, fraction = ""] = normalized.split(".");
  const cents = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
  return Number.isSafeInteger(cents) && cents > 0 ? cents : 0;
}

function formatUSD(cents: number) {
  return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "USD" }).format(cents / 100);
}

function initials(value: string) {
  return Array.from(value.trim()).slice(0, 2).join("").toUpperCase() || "?";
}
