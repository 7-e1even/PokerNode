import { useCallback, useEffect, useMemo, useState, type CSSProperties, type FormEvent, type ReactNode } from "react";
import {
  ArrowLeft, CircleDollarSign, Copy, Hash, KeyRound, Menu,
  PanelLeftClose, PanelLeftOpen, Plus, Radio, Server, Shuffle, Table2,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogMedia, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { api, post, remove } from "./api";
import PokerRoom from "./PokerRoom";
import type { Space, TableSeatSummary, TableSummary, User } from "./types";

interface Props {
  user: User;
  initialSpace: Space;
  initialTableID?: string;
  onBack: () => void;
  onNavigateTable: (tableID?: string) => void;
  onOpenBalances: () => void;
}

const sidebarStorageKey = "pokernode.channel-sidebar-collapsed.v2";

type RoomPlayer = TableSeatSummary & { tableID: string; tableName: string };

export default function ChannelRoom({ user, initialSpace, initialTableID, onBack, onNavigateTable, onOpenBalances }: Props) {
  const [tables, setTables] = useState<TableSummary[]>([]);
  const [selectedTable, setSelectedTable] = useState<TableSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [deletingTable, setDeletingTable] = useState<TableSummary | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => window.localStorage.getItem(sidebarStorageKey) !== "false");

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

  useEffect(() => {
    void loadTables(true);
    const timer = window.setInterval(() => void loadTables(), 5_000);
    return () => window.clearInterval(timer);
  }, [loadTables]);

  useEffect(() => {
    window.localStorage.setItem(sidebarStorageKey, String(sidebarCollapsed));
  }, [sidebarCollapsed]);

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
        }}
      />
    );
  }

  async function copyInvite() {
    await navigator.clipboard.writeText(initialSpace.invite_code || "");
    toast.success("频道邀请码已复制");
  }

  function openTable(table: TableSummary) {
    setMobileOpen(false);
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
          <Button className="lg:hidden" size="icon" variant="secondary" onClick={() => setMobileOpen(true)} aria-label="打开频道导航"><Menu /></Button>
          <button className="shrink-0 rounded-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={onBack} aria-label="返回 PokerNode 游戏大厅">
            <BrandMark className="size-10" aria-hidden="true" />
          </button>
          <Separator orientation="vertical" className="h-8" />
          <div className="min-w-0"><h1 className="truncate text-sm font-semibold">{initialSpace.name}</h1><p className="truncate text-xs text-muted-foreground">{tables.length} 桌 · {onlinePlayers} 人在线</p></div>
        </div>
        <div className="hidden items-center gap-2 sm:flex">{canManageBalances && <Button variant="outline" onClick={onOpenBalances}><CircleDollarSign data-icon="inline-start" />余额管理</Button>}{initialSpace.can_manage && initialSpace.invite_code && <Button variant="outline" onClick={() => void copyInvite()}><Copy data-icon="inline-start" />复制邀请码</Button>}</div>
      </header>

      <div className={cn("channel-workspace min-h-0 flex-1", sidebarCollapsed && "channel-workspace--collapsed")}>
        <ChannelNavigation
          className="hidden lg:flex"
          collapsed={sidebarCollapsed}
          space={initialSpace}
          tables={tables}
          onToggle={() => setSidebarCollapsed((current) => !current)}
          onBack={onBack}
          onOpenTable={openTable}
          onCopyInvite={() => void copyInvite()}
        />

        <main className="channel-main min-w-0">
          <section className="channel-table-panel flex min-w-0 flex-col">
            <div className="channel-table-toolbar flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-6">
              <div className="min-w-0"><div className="flex items-center gap-2"><Hash className="text-muted-foreground" /><strong className="truncate">频道选桌</strong><Badge variant="outline">{tables.length} 桌</Badge><Badge variant="outline">{onlinePlayers} 人</Badge></div><p className="mt-1 truncate text-xs text-muted-foreground">点击桌子进入 · 座位每 5 秒刷新</p></div>
              <div className="flex items-center gap-2">{canManageBalances && <Button className="sm:hidden" size="icon" variant="outline" onClick={onOpenBalances} aria-label="余额管理"><CircleDollarSign /></Button>}<Button variant="outline" disabled={!quickTable} onClick={() => quickTable && openTable(quickTable)}><Shuffle data-icon="inline-start" />快速开始</Button>{initialSpace.can_manage && <Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />新建牌桌</Button>}</div>
            </div>

            <div className="table-map-floor min-h-0 flex-1 overflow-auto p-4 sm:p-6">
              {error && <Alert variant="destructive" className="mb-4"><AlertDescription>{error}</AlertDescription></Alert>}
              {loading ? (
                <div className="table-map-grid">{Array.from({ length: 8 }, (_, index) => <TableSkeleton key={index} />)}</div>
              ) : tables.length === 0 ? (
                <Empty className="min-h-96 border bg-card"><EmptyHeader><EmptyMedia variant="icon"><Table2 /></EmptyMedia><EmptyTitle>房间里还没有牌桌</EmptyTitle><EmptyDescription>{initialSpace.can_manage ? "创建一张牌桌即可继续。" : "等待频道管理员创建牌桌。"}</EmptyDescription></EmptyHeader>{initialSpace.can_manage && <EmptyContent><Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />创建牌桌</Button></EmptyContent>}</Empty>
              ) : (
                <div className="table-map-grid">{tables.map((table) => <TableMapTile key={table.id} table={table} onOpen={() => openTable(table)} onDelete={initialSpace.can_manage ? () => setDeletingTable(table) : undefined} />)}{initialSpace.can_manage && <CreateTableTile onCreate={() => setCreateOpen(true)} />}</div>
              )}
            </div>
          </section>

          <RoomPanel space={initialSpace} players={roomPlayers} onOpenTable={(tableID) => {
            const table = tables.find((candidate) => candidate.id === tableID);
            if (table) openTable(table);
          }} />
        </main>
      </div>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-72 p-0" showCloseButton={false}>
          <SheetHeader className="border-b"><SheetTitle>{initialSpace.name}</SheetTitle><SheetDescription>选择频道牌桌</SheetDescription></SheetHeader>
          <ChannelNavigation className="flex min-h-0 flex-1 border-0" collapsed={false} space={initialSpace} tables={tables} onBack={onBack} onOpenTable={openTable} onCopyInvite={() => { setMobileOpen(false); void copyInvite(); }} />
        </SheetContent>
      </Sheet>

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

function ChannelNavigation({ className, collapsed, space, tables, onToggle, onBack, onOpenTable, onCopyInvite }: {
  className?: string;
  collapsed: boolean;
  space: Space;
  tables: TableSummary[];
  onToggle?: () => void;
  onBack: () => void;
  onOpenTable: (table: TableSummary) => void;
  onCopyInvite: () => void;
}) {
  return (
    <aside className={cn("channel-nav-pod min-h-0 flex-col overflow-hidden", className)}>
      {onToggle && (
        <>
          <div className={cn("flex min-h-12 items-center p-2", collapsed ? "justify-center" : "justify-between pl-4")}>
            {!collapsed && <span className="text-xs font-medium text-muted-foreground">频道导航</span>}
            <Tooltip><TooltipTrigger asChild><Button size="icon-sm" variant="ghost" onClick={onToggle} aria-label={collapsed ? "展开侧边栏" : "收起侧边栏"}>{collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}</Button></TooltipTrigger><TooltipContent side="right">{collapsed ? "展开侧边栏" : "收起侧边栏"}</TooltipContent></Tooltip>
          </div>
          <Separator />
        </>
      )}
      <div className="flex flex-col gap-1 p-2"><SidebarAction collapsed={collapsed} label="返回游戏大厅" icon={<ArrowLeft data-icon="inline-start" />} onClick={onBack} /></div>
      <Separator />
      {!collapsed && <div className="flex items-center justify-between gap-2 px-4 pt-4"><span className="text-xs font-medium text-muted-foreground">牌桌列表</span><Badge variant="outline">{tables.length}</Badge></div>}
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-1 p-3">
          {tables.length === 0 ? (!collapsed && <p className="px-2 py-6 text-center text-xs text-muted-foreground">还没有牌桌</p>) : tables.map((table) => (
            <Tooltip key={table.id}>
              <TooltipTrigger asChild>
                <Button className={cn("h-auto w-full min-w-0 py-2", collapsed ? "justify-center px-0" : "justify-start")} variant="ghost" onClick={() => onOpenTable(table)} aria-label={`${table.name}，${table.player_count} 人`}>
                  <span className="relative grid size-8 shrink-0 place-items-center rounded-lg bg-muted"><Table2 />{collapsed && table.player_count > 0 && <span className="absolute -right-1 -top-1 grid size-4 place-items-center rounded-full bg-foreground text-[0.625rem] text-background">{table.player_count}</span>}</span>
                  {!collapsed && <><span className="min-w-0 flex-1 text-left"><strong className="block truncate text-sm">{tableDisplayName(table.name)}</strong><small className="block truncate text-xs text-muted-foreground">{money(table.small_blind_cents)} / {money(table.big_blind_cents)}</small></span><Badge className="shrink-0" variant="outline">{table.player_count}</Badge></>}
                </Button>
              </TooltipTrigger>
              {collapsed && <TooltipContent side="right">{table.name} · {table.player_count}/{table.max_players} 人</TooltipContent>}
            </Tooltip>
          ))}
        </div>
      </ScrollArea>
      {!onToggle && space.can_manage && space.invite_code && (
        <div className="border-t p-3"><SidebarAction collapsed={collapsed} label="复制邀请码" icon={<Copy data-icon="inline-start" />} onClick={onCopyInvite} /></div>
      )}
    </aside>
  );
}

function SidebarAction({ collapsed, label, icon, onClick, variant = "ghost" }: {
  collapsed: boolean;
  label: string;
  icon: ReactNode;
  onClick?: () => void;
  variant?: "default" | "secondary" | "ghost";
}) {
  const control = <Button className={cn(collapsed ? "w-full px-0" : "w-full justify-start")} size={collapsed ? "icon" : "default"} variant={variant} onClick={onClick} aria-label={label}>{icon}{!collapsed && label}</Button>;
  if (!collapsed) return control;
  return <Tooltip><TooltipTrigger asChild>{control}</TooltipTrigger><TooltipContent side="right">{label}</TooltipContent></Tooltip>;
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
            <span className={cn("flex w-full items-center justify-between gap-2 text-xs text-muted-foreground", onDelete && "pr-7")}><span>{table.player_count > 0 ? "牌局进行中" : "等待玩家"}</span><span>{table.viewer_seated ? "你在此桌" : "点击进入"}</span></span>
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
  return <Button variant="ghost" className="table-map-tile items-center justify-center" onClick={onCreate}><Avatar size="lg"><AvatarFallback>+</AvatarFallback></Avatar><strong>创建新牌桌</strong><span className="text-xs text-muted-foreground">设置名称和盲注</span></Button>;
}

function RoomPanel({ space, players, onOpenTable }: { space: Space; players: RoomPlayer[]; onOpenTable: (tableID: string) => void }) {
  return (
    <aside className="channel-player-pod hidden min-h-0 flex-col 2xl:flex">
      <div className="flex min-h-16 shrink-0 items-center justify-between gap-3 px-4"><div><strong className="text-sm">房间玩家</strong><p className="text-xs text-muted-foreground">当前频道内已入座</p></div><Badge variant="outline">{players.length}</Badge></div>
      <ScrollArea className="min-h-0 flex-1">
        {players.length === 0 ? (
          <Empty className="border-0 py-12"><EmptyHeader><EmptyMedia variant="icon"><Avatar size="sm"><AvatarFallback>?</AvatarFallback></Avatar></EmptyMedia><EmptyTitle>暂无玩家</EmptyTitle><EmptyDescription>选择一张空桌坐下。</EmptyDescription></EmptyHeader></Empty>
        ) : (
          <div className="flex flex-col gap-1 p-2">{players.map((player) => <Button key={`${player.tableID}:${player.user_id}`} variant="ghost" className="h-auto justify-start py-2" onClick={() => onOpenTable(player.tableID)}><Avatar><AvatarFallback>{initials(player.name)}</AvatarFallback></Avatar><span className="min-w-0 flex-1 text-left"><strong className="block truncate text-sm">{player.name}</strong><small className="block truncate text-xs text-muted-foreground">{player.tableName} · {money(player.stack_cents)}</small></span></Button>)}</div>
        )}
      </ScrollArea>
      <Separator />
      <div className="flex flex-col gap-3 p-4 text-xs"><CompactStatus icon={<Server />} label="New API" value={hostOf(space.newapi_base_url)} /><CompactStatus icon={<KeyRound />} label="个人凭证" value={space.is_bound ? "已绑定" : "待绑定"} /><CompactStatus icon={<Radio />} label="实时服务" value="在线" /></div>
      <div className="p-3 pt-0"><Alert><CircleDollarSign /><AlertDescription>买入从当前频道余额扣除，离桌时返还剩余金额。</AlertDescription></Alert></div>
    </aside>
  );
}

function CreateTableDialog({ open, spaceID, onClose, onCreated }: { open: boolean; spaceID: string; onClose: () => void; onCreated: (table: TableSummary) => void }) {
  const [name, setName] = useState("");
  const [smallBlind, setSmallBlind] = useState("0.50");
  const [bigBlind, setBigBlind] = useState("1.00");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const small = Math.round(Number(smallBlind) * 100);
    const big = Math.round(Number(bigBlind) * 100);
    if (!Number.isFinite(small) || !Number.isFinite(big) || small <= 0 || big < small) {
      setError("请输入正确的大小盲金额");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await post<{ table: TableSummary }>(`/api/spaces/${spaceID}/tables`, { name, small_blind_cents: small, big_blind_cents: big });
      onCreated(result.table);
      setName("");
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
            <FieldDescription><CircleDollarSign />牌桌使用整数美分记账，不按 Token 数量展示。</FieldDescription>
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "创建中…" : "创建牌桌"}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function CompactStatus({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return <span className="flex min-w-0 items-center gap-2 text-muted-foreground">{icon}<span>{label}</span><strong className="max-w-52 truncate text-foreground">{value}</strong></span>;
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

function tableDisplayName(name: string) {
  return name.replace(/\s*·\s*\$\d+(?:\.\d+)?\s*\/\s*\$\d+(?:\.\d+)?\s*$/, "");
}

function hostOf(value: string) {
  try { return new URL(value).host; } catch { return value; }
}
