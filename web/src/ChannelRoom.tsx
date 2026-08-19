import { useCallback, useEffect, useMemo, useState, type CSSProperties, type FormEvent } from "react";
import {
  ArrowLeft, CircleDollarSign, Clock3, Copy, Crown, Link2, Plus, Shuffle, Table2,
  Trash2, Trophy,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogMedia, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { api, post, remove } from "./api";
import PokerRoom from "./PokerRoom";
import type { ChannelLeaderboardEntry, Space, TableSeatSummary, TableSummary, User } from "./types";

interface Props {
  user: User;
  initialSpace: Space;
  initialTableID?: string;
  onBack: () => void;
  onNavigateTable: (tableID?: string) => void;
  onOpenBindings: () => void;
  onOpenBalances: () => void;
}

type RoomPlayer = TableSeatSummary & { tableID: string; tableName: string };

export default function ChannelRoom({ user, initialSpace, initialTableID, onBack, onNavigateTable, onOpenBindings, onOpenBalances }: Props) {
  const [tables, setTables] = useState<TableSummary[]>([]);
  const [leaderboard, setLeaderboard] = useState<ChannelLeaderboardEntry[]>([]);
  const [selectedTable, setSelectedTable] = useState<TableSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [leaderboardLoading, setLeaderboardLoading] = useState(true);
  const [leaderboardError, setLeaderboardError] = useState("");
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [deletingTable, setDeletingTable] = useState<TableSummary | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);

  const loadTables = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true);
    try {
      const result = await api<{ tables: TableSummary[] }>(`/api/spaces/${initialSpace.id}/tables`);
      setTables(result.tables || []);
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "读取牌桌失败");
    } finally {
      if (showLoading) setLoading(false);
    }
  }, [initialSpace.id]);

  const loadLeaderboard = useCallback(async (showLoading = false) => {
    if (showLoading) setLeaderboardLoading(true);
    try {
      const result = await api<{ leaderboard: ChannelLeaderboardEntry[] }>(`/api/spaces/${initialSpace.id}/leaderboard`);
      setLeaderboard(Array.isArray(result.leaderboard) ? result.leaderboard : []);
      setLeaderboardError("");
    } catch (caught) {
      setLeaderboardError(caught instanceof Error ? caught.message : "读取频道排名失败");
    } finally {
      if (showLoading) setLeaderboardLoading(false);
    }
  }, [initialSpace.id]);

  useEffect(() => {
    void loadTables(true);
    const timer = window.setInterval(() => void loadTables(), 5_000);
    return () => window.clearInterval(timer);
  }, [loadTables]);

  useEffect(() => {
    void loadLeaderboard(true);
    const timer = window.setInterval(() => void loadLeaderboard(), 5_000);
    return () => window.clearInterval(timer);
  }, [loadLeaderboard]);

  useEffect(() => {
    if (!initialTableID) {
      setSelectedTable(null);
      return;
    }
    const target = tables.find((table) => table.id === initialTableID);
    if (target) setSelectedTable(target);
    else if (!loading) setError("该牌桌不存在或已不可用");
  }, [initialTableID, loading, tables]);

  const quickTable = useMemo(
    () => tables.find((table) => table.player_count > 0 && table.player_count < table.max_players)
      || tables.find((table) => table.player_count < table.max_players),
    [tables],
  );
  const onlinePlayers = tables.reduce((total, table) => total + table.player_count, 0);
  const canManageBalances = initialSpace.is_owner || !!user.permissions?.includes("balances:manage");
  const roomPlayers = useMemo<RoomPlayer[]>(
    () => tables.flatMap((table) => (table.players || []).map((player) => ({ ...player, tableID: table.id, tableName: table.name }))),
    [tables],
  );

  if (selectedTable) {
    return (
      <PokerRoom
        user={user}
        initialSpace={initialSpace}
        initialTable={selectedTable}
        onBack={() => {
          setSelectedTable(null);
          onNavigateTable();
          void loadTables();
          void loadLeaderboard();
        }}
      />
    );
  }

  async function copyInvite() {
    await navigator.clipboard.writeText(initialSpace.invite_code || "");
    toast.success("频道邀请码已复制");
  }

  function openTable(table: TableSummary) {
    setSelectedTable(table);
    onNavigateTable(table.id);
  }

  async function deleteSelectedTable() {
    if (!deletingTable || deletingTable.player_count > 0) return;
    setDeleteBusy(true);
    try {
      await remove(`/api/spaces/${initialSpace.id}/tables/${deletingTable.id}`);
      setTables((current) => current.filter((table) => table.id !== deletingTable.id));
      setDeletingTable(null);
      toast.success("牌桌已删除");
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "删除牌桌失败");
    } finally {
      setDeleteBusy(false);
    }
  }

  return (
    <div className="game-canvas channel-shell flex min-h-svh flex-col">
      <header className="game-topbar flex h-16 shrink-0 items-center justify-between gap-4 px-3 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <Button variant="ghost" onClick={onBack}><ArrowLeft data-icon="inline-start" /><span className="hidden sm:inline">频道大厅</span></Button>
          <Separator orientation="vertical" className="h-8" />
          <BrandMark className="size-9 shrink-0" aria-hidden="true" />
          <div className="min-w-0"><h1 className="truncate text-sm font-semibold">{initialSpace.name}</h1><p className="truncate text-xs text-muted-foreground">{tables.length} 个牌桌 · {onlinePlayers} 人在线</p></div>
        </div>
        <div className="flex items-center gap-2"><Button size="sm" variant="outline" onClick={onOpenBindings}><Link2 data-icon="inline-start" /><span className="hidden sm:inline">频道账号</span></Button>{canManageBalances && <Button size="sm" variant="outline" onClick={onOpenBalances}><CircleDollarSign data-icon="inline-start" /><span className="hidden sm:inline">余额管理</span></Button>}{initialSpace.can_manage && initialSpace.invite_code && <Button size="sm" variant="outline" onClick={() => void copyInvite()}><Copy data-icon="inline-start" /><span className="hidden sm:inline">邀请码</span></Button>}</div>
      </header>

      <main className="min-w-0 flex-1 overflow-auto">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end" aria-labelledby="channel-title">
            <div className="flex max-w-2xl flex-col gap-2">
              <Badge variant="outline" className="w-fit">频道牌局</Badge>
              <h2 id="channel-title" className="font-heading text-3xl font-semibold tracking-tight sm:text-4xl">{initialSpace.name}</h2>
              <p className="text-sm leading-6 text-muted-foreground">选择牌桌直接进入游戏；右侧排名会计入当前桌上的筹码。</p>
            </div>
            <div className="flex gap-2"><Badge variant="secondary">{tables.length} 个牌桌</Badge><Badge variant="outline">{onlinePlayers} 人在线</Badge></div>
          </section>

          <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
            <Card className="min-w-0 [--card-spacing:--spacing(5)]">
              <CardHeader>
                <CardTitle>牌局</CardTitle>
                <CardDescription>选择一张牌桌坐下，空位和玩家状态每 5 秒更新。</CardDescription>
                <CardAction className="flex gap-2"><Button variant="outline" disabled={!quickTable} onClick={() => quickTable && openTable(quickTable)}><Shuffle data-icon="inline-start" />快速加入</Button>{initialSpace.can_manage && <Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />新建牌桌</Button>}</CardAction>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
              {loading ? (
                  <div className="table-map-grid">{Array.from({ length: 6 }, (_, index) => <TableSkeleton key={index} />)}</div>
              ) : tables.length === 0 ? (
                  <Empty className="min-h-80 border"><EmptyHeader><EmptyMedia variant="icon"><Table2 /></EmptyMedia><EmptyTitle>还没有牌桌</EmptyTitle><EmptyDescription>{initialSpace.can_manage ? "创建第一张牌桌，频道就可以开局了。" : "频道管理员还没有创建牌桌。"}</EmptyDescription></EmptyHeader>{initialSpace.can_manage && <EmptyContent><Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />创建牌桌</Button></EmptyContent>}</Empty>
              ) : (
                  <div className="table-map-grid">{tables.map((table) => <TableMapTile key={table.id} table={table} onOpen={() => openTable(table)} onDelete={initialSpace.can_manage ? () => setDeletingTable(table) : undefined} />)}{initialSpace.can_manage && <CreateTableTile onCreate={() => setCreateOpen(true)} />}</div>
              )}
              </CardContent>
            </Card>

            <LeaderboardCard entries={leaderboard} players={roomPlayers} currentUserID={user.id} loading={leaderboardLoading} error={leaderboardError} />
          </div>
        </div>
      </main>

      <CreateTableDialog
        open={createOpen}
        spaceID={initialSpace.id}
        onClose={() => setCreateOpen(false)}
        onCreated={(table) => {
          setTables((current) => [table, ...current]);
          setCreateOpen(false);
          toast.success("牌桌已创建");
        }}
      />

      <AlertDialog open={deletingTable !== null} onOpenChange={(open) => !open && !deleteBusy && setDeletingTable(null)}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogMedia><Trash2 /></AlertDialogMedia>
            <AlertDialogTitle>删除“{deletingTable?.name}”？</AlertDialogTitle>
            <AlertDialogDescription>
              {deletingTable && deletingTable.player_count > 0
                ? `当前还有 ${deletingTable.player_count} 名玩家，需全部结算离桌后才能删除。`
                : "牌桌及其牌局状态将永久删除，已有资金流水仍会保留。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteBusy}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={deleteBusy || (deletingTable?.player_count || 0) > 0} onClick={(event) => { event.preventDefault(); void deleteSelectedTable(); }}>
              {deleteBusy && <Spinner data-icon="inline-start" />}{deleteBusy ? "正在删除…" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function TableMapTile({ table, onOpen, onDelete }: { table: TableSummary; onOpen: () => void; onDelete?: () => void }) {
  const available = table.player_count < table.max_players;
  const players = [...(table.players || [])].sort((a, b) => a.seat - b.seat);
  const playerNames = players.map((player) => player.name).join("、");
  return (
    <div className="table-map-item group">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" className="table-map-tile" onClick={onOpen} aria-label={`${table.name}，${table.player_count}/${table.max_players} 人${playerNames ? `，玩家 ${playerNames}` : "，空桌"}`}>
            <span className="flex w-full min-w-0 items-center justify-between gap-2"><strong className="truncate text-sm">{tableDisplayName(table.name)}</strong><Badge className="shrink-0" variant={table.viewer_seated ? "default" : "outline"}>{table.player_count}/{table.max_players}</Badge></span>
            <span className="table-map-stage">
              <span className="table-map-surface"><strong>{money(table.small_blind_cents)}/{money(table.big_blind_cents)}</strong><small>{available ? (table.player_count > 0 ? `${table.player_count} 人入座` : "等待入座") : "已满"}</small></span>
              {players.map((player, index) => <MapSeat key={player.user_id} player={player} style={mapSeatStyle(index, players.length)} />)}
            </span>
            <span className={cn("flex w-full items-center justify-between gap-2 text-xs text-muted-foreground", onDelete && "pr-7")}><span className="truncate">{table.player_count > 0 ? "牌局进行中" : "等待玩家"} · {table.action_timeout_seconds} 秒</span><span className="shrink-0">{table.viewer_seated ? "你在此桌" : "点击进入"}</span></span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{playerNames ? `${playerNames} 正在这桌` : "空桌，点击入座"}</TooltipContent>
      </Tooltip>
      {onDelete && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="icon-xs" variant="ghost" className="absolute right-2 bottom-2 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100" onClick={onDelete} aria-label={`删除牌桌 ${table.name}`}><Trash2 /></Button>
          </TooltipTrigger>
          <TooltipContent side="top">{table.player_count > 0 ? "有玩家时不能删除" : "删除牌桌"}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function MapSeat({ player, style }: { player: TableSeatSummary; style: CSSProperties }) {
  return <span className="table-map-seat -translate-x-1/2 -translate-y-1/2" style={style}><Avatar size="sm"><AvatarFallback>{initials(player.name)}</AvatarFallback></Avatar><small>{player.name}</small></span>;
}

function mapSeatStyle(index: number, count: number): CSSProperties {
  const angle = (90 + index * 360 / count) * Math.PI / 180;
  return {
    left: `${50 + 44 * Math.cos(angle)}%`,
    top: `${50 + 42 * Math.sin(angle)}%`,
  };
}

function CreateTableTile({ onCreate }: { onCreate: () => void }) {
  return <Button variant="ghost" className="table-map-tile items-center justify-center" onClick={onCreate}><Avatar size="lg"><AvatarFallback>+</AvatarFallback></Avatar><strong>创建新牌桌</strong><span className="text-xs text-muted-foreground">设置名称、盲注和行动时限</span></Button>;
}

function LeaderboardCard({ entries, players, currentUserID, loading, error }: {
  entries: ChannelLeaderboardEntry[];
  players: RoomPlayer[];
  currentUserID: number;
  loading: boolean;
  error: string;
}) {
  const activePlayers = new Map(players.map((player) => [player.user_id, player]));
  const ranked = entries
    .map((entry) => ({ ...entry, currentNetCents: entry.net_cents + (activePlayers.get(entry.user_id)?.stack_cents || 0) }))
    .sort((left, right) => right.currentNetCents - left.currentNetCents || right.sessions - left.sessions || left.display_name.localeCompare(right.display_name));

  return (
    <Card className="[--card-spacing:--spacing(5)] xl:sticky xl:top-6">
      <CardHeader>
        <CardTitle>频道排名</CardTitle>
        <CardDescription>按当前净胜筹码排序</CardDescription>
        <CardAction><Trophy /></CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
        {loading ? (
          <div className="flex flex-col gap-3" aria-hidden="true">{Array.from({ length: 5 }, (_, index) => <div className="flex items-center gap-3" key={index}><Skeleton className="size-7 rounded-full" /><Skeleton className="size-9 rounded-full" /><div className="grid flex-1 gap-2"><Skeleton className="h-4 w-24" /><Skeleton className="h-3 w-16" /></div><Skeleton className="h-5 w-16" /></div>)}</div>
        ) : ranked.length === 0 ? (
          <Empty className="border-0 py-10"><EmptyHeader><EmptyMedia variant="icon"><Trophy /></EmptyMedia><EmptyTitle>暂无排名</EmptyTitle><EmptyDescription>完成第一场牌局后会显示战绩。</EmptyDescription></EmptyHeader></Empty>
        ) : (
          <ScrollArea className="max-h-[36rem]">
            <div className="flex flex-col gap-1 pr-3">
              {ranked.map((entry, index) => {
                const active = activePlayers.get(entry.user_id);
                return (
                  <div className="flex items-center gap-3 rounded-lg px-2 py-3" key={entry.user_id}>
                    <Badge className="size-7 shrink-0 justify-center rounded-full p-0" variant={index === 0 ? "default" : "outline"}>{index === 0 ? <Crown /> : index + 1}</Badge>
                    <Avatar><AvatarFallback>{initials(entry.display_name)}</AvatarFallback></Avatar>
                    <div className="min-w-0 flex-1"><div className="flex items-center gap-2"><strong className="truncate text-sm">{entry.display_name}</strong>{entry.user_id === currentUserID && <Badge variant="secondary">我</Badge>}</div><p className="truncate text-xs text-muted-foreground">{active ? `${tableDisplayName(active.tableName)} · 桌上 ${money(active.stack_cents)}` : entry.sessions > 0 ? `${entry.sessions} 次买入` : "尚未开局"}</p></div>
                    <strong className="shrink-0 tabular-nums">{netMoney(entry.currentNetCents)}</strong>
                  </div>
                );
              })}
            </div>
          </ScrollArea>
        )}
        <Separator />
        <p className="text-xs leading-5 text-muted-foreground">净胜由已完成的买入和结算计算；正在牌桌上的筹码会实时计入。</p>
      </CardContent>
    </Card>
  );
}

function CreateTableDialog({ open, spaceID, onClose, onCreated }: { open: boolean; spaceID: string; onClose: () => void; onCreated: (table: TableSummary) => void }) {
  const [name, setName] = useState("");
  const [smallBlind, setSmallBlind] = useState("0.50");
  const [bigBlind, setBigBlind] = useState("1.00");
  const [actionTimeout, setActionTimeout] = useState("25");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const small = Math.round(Number(smallBlind) * 100);
    const big = Math.round(Number(bigBlind) * 100);
    const timeout = Number(actionTimeout);
    if (!Number.isFinite(small) || !Number.isFinite(big) || small <= 0 || big < small) {
      setError("请输入正确的大小盲金额");
      return;
    }
    if (!Number.isInteger(timeout) || timeout < 5 || timeout > 300) {
      setError("行动时限需为 5–300 秒之间的整数");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await post<{ table: TableSummary }>(`/api/spaces/${spaceID}/tables`, { name, small_blind_cents: small, big_blind_cents: big, action_timeout_seconds: timeout });
      onCreated(result.table);
      setName("");
      setActionTimeout("25");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "创建牌桌失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>创建牌桌</DialogTitle><DialogDescription>新牌桌使用当前频道的 New API 连接和成员体系。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <Field><FieldLabel htmlFor="table-name">牌桌名称</FieldLabel><Input id="table-name" name="table-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：休闲桌…" autoComplete="off" required autoFocus /></Field>
            <div className="grid grid-cols-2 gap-4">
              <Field><FieldLabel htmlFor="small-blind">小盲（美元）</FieldLabel><Input id="small-blind" name="small-blind" type="number" min="0.01" step="0.01" value={smallBlind} onChange={(event) => setSmallBlind(event.target.value)} required /></Field>
              <Field><FieldLabel htmlFor="big-blind">大盲（美元）</FieldLabel><Input id="big-blind" name="big-blind" type="number" min="0.01" step="0.01" value={bigBlind} onChange={(event) => setBigBlind(event.target.value)} required /></Field>
            </div>
            <Field data-invalid={error.startsWith("行动时限") || undefined}>
              <FieldLabel htmlFor="action-timeout">行动时限（秒）</FieldLabel>
              <Input id="action-timeout" name="action-timeout" type="number" min="5" max="300" step="1" inputMode="numeric" value={actionTimeout} onChange={(event) => setActionTimeout(event.target.value)} aria-invalid={error.startsWith("行动时限") || undefined} required />
              <FieldDescription><Clock3 />每位玩家每次行动的倒计时；超时能过牌则自动过牌，否则自动弃牌。</FieldDescription>
            </Field>
            <FieldDescription><CircleDollarSign />牌桌使用整数美分记账，不按 Token 数量展示。</FieldDescription>
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "创建中…" : "创建牌桌"}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function TableSkeleton() {
  return <div className="table-map-tile pointer-events-none"><Skeleton className="h-4 w-2/3" /><Skeleton className="aspect-[880/493] w-36 max-w-full self-center rounded-[999px]" /><Skeleton className="h-3 w-full" /></div>;
}

function initials(name: string) {
  return name.trim().slice(0, 2).toUpperCase();
}

function money(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2 }).format(cents / 100);
}

function netMoney(cents: number) {
  return cents > 0 ? `+${money(cents)}` : money(cents);
}

function tableDisplayName(name: string) {
  return name.replace(/\s*·\s*\$\d+(?:\.\d+)?\s*\/\s*\$\d+(?:\.\d+)?\s*$/, "");
}
