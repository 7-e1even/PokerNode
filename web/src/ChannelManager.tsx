import { useEffect, useRef, useState, type FormEvent } from "react";
import { ArrowRight, Copy, Eraser, KeyRound, RadioTower, RefreshCw, Server, Table2, Trash2, UsersRound, WalletCards } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, post, put, remove } from "./api";
import type { AdminSpaceSummary, ChannelMember, Space, TableSummary } from "./types";

type ChannelDetail = {
  space: Space;
  members: ChannelMember[];
  tables: TableSummary[];
};

type TableAction = {
  mode: "clear" | "delete";
  table: TableSummary;
};

export default function ChannelManager({ spaces, canManageBalances, onSpaceChanged }: {
  spaces: AdminSpaceSummary[];
  canManageBalances: boolean;
  onSpaceChanged: (space: AdminSpaceSummary) => void;
}) {
  const [selected, setSelected] = useState<AdminSpaceSummary | null>(null);
  const [detail, setDetail] = useState<ChannelDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [token, setToken] = useState("");
  const [quota, setQuota] = useState(500000);
  const [connectionBusy, setConnectionBusy] = useState(false);
  const [tableAction, setTableAction] = useState<TableAction | null>(null);
  const [tableBusy, setTableBusy] = useState(false);
  const loadSequence = useRef(0);

  async function loadChannel(space: AdminSpaceSummary, showLoading = true) {
    const sequence = ++loadSequence.current;
    if (showLoading) setLoading(true);
    setError("");
    try {
      const [spaceResult, memberResult, tableResult] = await Promise.all([
        api<{ space: Space }>(`/api/spaces/${space.id}`),
        api<{ members: ChannelMember[] }>(`/api/spaces/${space.id}/members`),
        api<{ tables: TableSummary[] }>(`/api/spaces/${space.id}/tables`),
      ]);
      if (sequence !== loadSequence.current) return;
      setDetail({
        space: spaceResult.space,
        members: memberResult.members || [],
        tables: tableResult.tables || [],
      });
      setBaseURL(spaceResult.space.newapi_base_url);
      setQuota(spaceResult.space.quota_per_usd);
      setToken("");
    } catch (caught) {
      if (sequence === loadSequence.current) setError(caught instanceof Error ? caught.message : "读取频道详情失败");
    } finally {
      if (showLoading && sequence === loadSequence.current) setLoading(false);
    }
  }

  useEffect(() => {
    if (!selected) {
      setDetail(null);
      setError("");
      return;
    }
    setBaseURL(selected.newapi_base_url);
    setQuota(selected.quota_per_usd);
    setToken("");
    void loadChannel(selected);
  }, [selected?.id]);

  function updateSummary(patch: Partial<AdminSpaceSummary>) {
    if (!selected) return;
    const updated = { ...selected, ...patch };
    setSelected(updated);
    onSpaceChanged(updated);
  }

  async function saveConnection(event: FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setConnectionBusy(true);
    try {
      const result = await put<{ space: Space }>(`/api/spaces/${selected.id}/connection`, {
        newapi_base_url: baseURL,
        admin_token: token,
        quota_per_usd: quota,
      });
      setDetail((current) => current ? { ...current, space: result.space } : current);
      updateSummary({
        newapi_base_url: result.space.newapi_base_url,
        admin_token_last4: result.space.admin_token_last4,
        quota_per_usd: result.space.quota_per_usd,
      });
      setToken("");
      toast.success("频道节点设置已保存");
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "频道节点设置保存失败");
    } finally {
      setConnectionBusy(false);
    }
  }

  async function copyInvite() {
    if (!detail?.space.invite_code) return;
    await navigator.clipboard.writeText(detail.space.invite_code);
    toast.success("频道邀请码已复制");
  }

  async function confirmTableAction() {
    if (!selected || !tableAction) return;
    setTableBusy(true);
    try {
      if (tableAction.mode === "clear") {
        const result = await post<{ settled_players: number; settled_cents: number; table: TableSummary }>(`/api/spaces/${selected.id}/tables/${tableAction.table.id}/clear`);
        setDetail((current) => current ? { ...current, tables: current.tables.map((table) => table.id === result.table.id ? result.table : table) } : current);
        toast.success(`已结算并移出 ${result.settled_players} 名玩家，共 ${money(result.settled_cents)}`);
      } else {
        const force = tableAction.table.player_count > 0 ? "?force=true" : "";
        await remove(`/api/spaces/${selected.id}/tables/${tableAction.table.id}${force}`);
        setDetail((current) => current ? { ...current, tables: current.tables.filter((table) => table.id !== tableAction.table.id) } : current);
        updateSummary({ table_count: Math.max(0, selected.table_count - 1) });
        toast.success(tableAction.table.player_count > 0 ? "玩家已结算，牌桌已删除" : "牌桌已删除");
      }
      setTableAction(null);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : tableAction.mode === "clear" ? "清空牌桌失败" : "删除牌桌失败");
    } finally {
      setTableBusy(false);
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>频道节点</CardTitle>
          <CardDescription>选择频道可管理节点连接、邀请码、成员绑定和牌桌；管理员凭证仍只显示末四位。</CardDescription>
          <CardAction><Badge variant="outline">{spaces.length} 个节点</Badge></CardAction>
        </CardHeader>
        <CardContent>
          {spaces.length === 0 ? (
            <Empty className="min-h-64"><EmptyHeader><EmptyMedia variant="icon"><RadioTower /></EmptyMedia><EmptyTitle>还没有可管理频道</EmptyTitle><EmptyDescription>创建频道或为账号分配已加入的频道后，会在这里显示。</EmptyDescription></EmptyHeader></Empty>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader><TableRow><TableHead>频道 / New API</TableHead><TableHead>负责人</TableHead><TableHead>成员绑定</TableHead><TableHead>牌桌</TableHead><TableHead>资金流水</TableHead><TableHead>状态</TableHead><TableHead className="w-20 text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>{spaces.map((space) => (
                  <TableRow key={space.id}>
                    <TableCell><Button variant="ghost" className="h-auto max-w-72 justify-start px-0 py-1 text-left" onClick={() => setSelected(space)}><span className="min-w-0 flex-1"><strong className="block truncate">{space.name}</strong><small className="block truncate text-muted-foreground">{hostOf(space.newapi_base_url)} · Token …{space.admin_token_last4}</small></span><ArrowRight data-icon="inline-end" /></Button></TableCell>
                    <TableCell><span className="block">{space.owner_display_name}</span><small className="text-muted-foreground">@{space.owner_username}</small></TableCell>
                    <TableCell className="tabular-nums">{space.bound_member_count} / {space.member_count}</TableCell>
                    <TableCell className="tabular-nums">{space.table_count}</TableCell>
                    <TableCell className="tabular-nums">{space.operation_count}</TableCell>
                    <TableCell><ChannelStatus space={space} /></TableCell>
                    <TableCell className="text-right"><Button size="sm" variant="outline" onClick={() => setSelected(space)}>管理</Button></TableCell>
                  </TableRow>
                ))}</TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <Sheet open={Boolean(selected)} onOpenChange={(open) => !open && !connectionBusy && !tableBusy && setSelected(null)}>
        <SheetContent className="data-[side=right]:w-full data-[side=right]:sm:max-w-2xl">
          <SheetHeader className="border-b pr-12">
            <SheetTitle>{selected?.name || "频道详情"}</SheetTitle>
            <SheetDescription>{selected ? `${hostOf(selected.newapi_base_url)} · 负责人 @${selected.owner_username}` : "管理频道节点与牌桌"}</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-4">
            {error && <Alert variant="destructive" className="mb-4"><AlertDescription>{error}</AlertDescription></Alert>}
            {loading ? <ChannelDetailSkeleton /> : detail && selected ? (
              <div className="flex flex-col gap-4">
                <div className="grid gap-3 sm:grid-cols-3">
                  <MiniMetric icon={<UsersRound />} label="成员绑定" value={`${selected.bound_member_count}/${selected.member_count}`} />
                  <MiniMetric icon={<Table2 />} label="牌桌" value={`${detail.tables.length}`} />
                  <MiniMetric icon={<WalletCards />} label="资金流水" value={`${selected.operation_count}`} />
                </div>

                {selected.failed_operations > 0 && <Alert variant="destructive"><AlertDescription className="flex flex-wrap items-center justify-between gap-2"><span>当前频道有 {selected.failed_operations} 条失败资金流水，请核对对应成员余额。</span>{canManageBalances && <Button size="xs" variant="outline" asChild><a href="/admin/balances">打开余额管理</a></Button>}</AlertDescription></Alert>}

                <Card size="sm">
                  <CardHeader><CardTitle>成员邀请码</CardTitle><CardDescription>只向需要加入当前频道的成员发送此邀请码。</CardDescription><CardAction><Button size="sm" variant="outline" disabled={!detail.space.invite_code} onClick={() => void copyInvite()}><Copy data-icon="inline-start" />{detail.space.invite_code ? "复制" : "不可用"}</Button></CardAction></CardHeader>
                  <CardContent><code className="block rounded-lg bg-muted px-3 py-2 font-mono text-sm">{detail.space.invite_code || "当前账号无权查看"}</code></CardContent>
                </Card>

                <Card size="sm">
                  <CardHeader><CardTitle>New API 节点</CardTitle><CardDescription>保存时会在线验证新管理员凭证，原密文不会返回前端。</CardDescription></CardHeader>
                  <CardContent>
                    <form onSubmit={saveConnection}>
                      <FieldGroup>
                        <Field><FieldLabel htmlFor="channel-node-url"><Server />New API 地址</FieldLabel><Input id="channel-node-url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} required /></Field>
                        <Field><FieldLabel htmlFor="channel-node-token"><KeyRound />新管理员凭证</FieldLabel><Input id="channel-node-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder={`当前 ····${selected.admin_token_last4}`} required /><FieldDescription>只在保存时使用，并继续加密存储。</FieldDescription></Field>
                        <Field><FieldLabel htmlFor="channel-node-quota">每美元对应 quota</FieldLabel><Input id="channel-node-quota" type="number" min={100} step={100} value={quota} onChange={(event) => setQuota(Number(event.target.value))} required /><FieldDescription>默认 500,000 quota = $1.00。</FieldDescription></Field>
                        <Button className="self-start" disabled={connectionBusy || !baseURL.trim() || !token.trim() || quota < 100}>{connectionBusy && <Spinner data-icon="inline-start" />}{connectionBusy ? "正在验证…" : "验证并保存"}</Button>
                      </FieldGroup>
                    </form>
                  </CardContent>
                </Card>

                <Card size="sm">
                  <CardHeader><CardTitle>成员绑定</CardTitle><CardDescription>{detail.members.length} 名成员；个人凭证仍由成员本人绑定。</CardDescription></CardHeader>
                  <CardContent>{detail.members.length === 0 ? <Empty className="min-h-40"><EmptyHeader><EmptyMedia variant="icon"><UsersRound /></EmptyMedia><EmptyTitle>还没有成员</EmptyTitle><EmptyDescription>成员使用邀请码加入后会显示在这里。</EmptyDescription></EmptyHeader></Empty> : (
                    <div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>成员</TableHead><TableHead>New API 账号</TableHead><TableHead>状态</TableHead></TableRow></TableHeader><TableBody>{detail.members.map((member) => <TableRow key={member.user_id}>
                      <TableCell className="max-w-24"><strong className="block truncate">{member.poker_display_name}</strong></TableCell>
                      <TableCell className="max-w-32 sm:max-w-none"><span className="block truncate">{member.newapi_display_name || member.newapi_username || "—"}</span>{member.newapi_username && <small className="block truncate text-muted-foreground">@{member.newapi_username}</small>}</TableCell>
                      <TableCell><Badge variant={member.newapi_user_id ? "secondary" : "outline"}>{member.newapi_user_id ? <><span className="sm:hidden">已绑定</span><span className="hidden sm:inline">已绑定 ····{member.user_token_last4 || ""}</span></> : "待绑定"}</Badge></TableCell>
                    </TableRow>)}</TableBody></Table></div>
                  )}</CardContent>
                </Card>

                <Card size="sm">
                  <CardHeader><CardTitle>牌桌管理</CardTitle><CardDescription>可清空等待中的牌桌，或在确认结算后永久删除。</CardDescription><CardAction><Button size="sm" variant="outline" disabled={loading} onClick={() => void loadChannel(selected)}><RefreshCw data-icon="inline-start" />刷新</Button></CardAction></CardHeader>
                  <CardContent>{detail.tables.length === 0 ? <Empty className="min-h-40"><EmptyHeader><EmptyMedia variant="icon"><Table2 /></EmptyMedia><EmptyTitle>当前没有牌桌</EmptyTitle><EmptyDescription>请在频道大厅创建第一张牌桌。</EmptyDescription></EmptyHeader></Empty> : (
                    <div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>牌桌</TableHead><TableHead>玩家</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{detail.tables.map((table) => {
                      const active = tableHandActive(table);
                      return <TableRow key={table.id}><TableCell><strong className="block">{table.name}</strong><small className="text-muted-foreground">{table.game_type === "landlord" ? "斗地主" : "德州扑克"}</small></TableCell><TableCell className="tabular-nums">{table.player_count}/{table.max_players}</TableCell><TableCell><Badge variant={active ? "outline" : "secondary"}>{active ? "牌局进行中" : "等待中"}</Badge></TableCell><TableCell><div className="flex justify-end gap-1"><Button size="icon-sm" variant="ghost" disabled={active || table.player_count === 0} onClick={() => setTableAction({ mode: "clear", table })} aria-label={`清空牌桌 ${table.name}`}><Eraser /></Button><Button size="icon-sm" variant="ghost" disabled={active} onClick={() => setTableAction({ mode: "delete", table })} aria-label={`删除牌桌 ${table.name}`}><Trash2 /></Button></div></TableCell></TableRow>;
                    })}</TableBody></Table></div>
                  )}</CardContent>
                </Card>
              </div>
            ) : !error && <Empty className="min-h-64"><EmptyHeader><EmptyMedia variant="icon"><RadioTower /></EmptyMedia><EmptyTitle>频道详情不可用</EmptyTitle><EmptyDescription>请关闭后重新打开该频道。</EmptyDescription></EmptyHeader></Empty>}
          </div>
        </SheetContent>
      </Sheet>

      <AlertDialog open={Boolean(tableAction)} onOpenChange={(open) => !open && !tableBusy && setTableAction(null)}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader><AlertDialogMedia>{tableAction?.mode === "clear" ? <Eraser /> : <Trash2 />}</AlertDialogMedia><AlertDialogTitle>{tableAction?.mode === "clear" ? `清空“${tableAction.table.name}”？` : `删除“${tableAction?.table.name}”？`}</AlertDialogTitle><AlertDialogDescription>{tableAction?.mode === "clear" ? `将依次结算并移出当前 ${tableAction.table.player_count} 名玩家，牌桌配置保持不变。` : tableAction && tableAction.table.player_count > 0 ? `将先结算并移出当前 ${tableAction.table.player_count} 名玩家，再永久删除牌桌；资金流水仍会保留。` : "牌桌及当前牌局状态将永久删除，已有资金流水仍会保留。"}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel disabled={tableBusy}>取消</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={tableBusy} onClick={(event) => { event.preventDefault(); void confirmTableAction(); }}>{tableBusy && <Spinner data-icon="inline-start" />}{tableBusy ? "正在处理…" : tableAction?.mode === "clear" ? "确认清空" : "确认删除"}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function ChannelStatus({ space }: { space: AdminSpaceSummary }) {
  if (space.failed_operations > 0) return <Badge variant="destructive">{space.failed_operations} 条失败</Badge>;
  if (space.bound_member_count < space.member_count) return <Badge variant="outline">待绑定</Badge>;
  return <Badge variant="secondary">正常</Badge>;
}

function MiniMetric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return <Card size="sm"><CardHeader className="flex flex-row items-center gap-3"><span className="grid size-9 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">{icon}</span><span><CardDescription>{label}</CardDescription><CardTitle className="text-lg tabular-nums">{value}</CardTitle></span></CardHeader></Card>;
}

function ChannelDetailSkeleton() {
  return <div className="flex flex-col gap-4"><div className="grid gap-3 sm:grid-cols-3">{Array.from({ length: 3 }, (_, index) => <Skeleton key={index} className="h-20" />)}</div>{Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-44" />)}</div>;
}

function tableHandActive(table: TableSummary) {
  return table.street === "preflop" || table.street === "flop" || table.street === "turn" || table.street === "river" || table.street === "bidding" || table.street === "playing";
}

function hostOf(value: string) {
  try { return new URL(value).host; } catch { return value; }
}

function money(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2 }).format(cents / 100);
}
