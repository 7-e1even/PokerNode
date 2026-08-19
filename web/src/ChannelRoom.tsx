import { useCallback, useEffect, useMemo, useState, type CSSProperties, type FormEvent } from "react";
import {
  ArrowLeft, CircleDollarSign, Clock3, Copy, Crown, Eraser, Link2, Plus, Shuffle, Table2,
  Spade, Trash2, Trophy,
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
import LandlordRoom from "./LandlordRoom";
import PokerRoom from "./PokerRoom";
import type { ChannelLeaderboardEntry, GameType, Space, TableSeatSummary, TableSummary, User } from "./types";

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
type TableAdminAction = { mode: "clear" | "delete"; table: TableSummary };

export default function ChannelRoom({ user, initialSpace, initialTableID, onBack, onNavigateTable, onOpenBindings, onOpenBalances }: Props) {
  const [tables, setTables] = useState<TableSummary[]>([]);
  const [leaderboard, setLeaderboard] = useState<ChannelLeaderboardEntry[]>([]);
  const [selectedTable, setSelectedTable] = useState<TableSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [leaderboardLoading, setLeaderboardLoading] = useState(true);
  const [leaderboardError, setLeaderboardError] = useState("");
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [gameType, setGameType] = useState<GameType>("texas_holdem");
  const [tableAdminAction, setTableAdminAction] = useState<TableAdminAction | null>(null);
  const [tableActionBusy, setTableActionBusy] = useState(false);

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
    if (target) {
      setGameType(target.game_type);
      setSelectedTable(target);
    }
    else if (!loading) setError("该牌桌不存在或已不可用");
  }, [initialTableID, loading, tables]);

  const gameTables = useMemo(() => tables.filter((table) => table.game_type === gameType), [gameType, tables]);
  const viewerTable = useMemo(() => tables.find((table) => table.viewer_seated), [tables]);
  const quickTable = useMemo(
    () => viewerTable
      || gameTables.find((table) => table.player_count > 0 && table.player_count < table.max_players)
      || gameTables.find((table) => table.player_count < table.max_players),
    [gameTables, viewerTable],
  );
  const onlinePlayers = tables.reduce((total, table) => total + table.player_count, 0);
  const canManageBalances = initialSpace.is_owner || !!user.permissions?.includes("balances:manage");
  const roomPlayers = useMemo<RoomPlayer[]>(
    () => tables.flatMap((table) => (table.players || []).map((player) => ({ ...player, tableID: table.id, tableName: table.name }))),
    [tables],
  );

  if (selectedTable) {
    if (selectedTable.game_type === "landlord") {
      return (
        <LandlordRoom
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
    setGameType(table.game_type);
    setSelectedTable(table);
    onNavigateTable(table.id);
  }

  async function runTableAdminAction() {
    if (!tableAdminAction) return;
    const { mode, table } = tableAdminAction;
    setTableActionBusy(true);
    try {
      if (mode === "clear") {
        const result = await post<{ settled_players: number; settled_cents: number; table: TableSummary }>(`/api/spaces/${initialSpace.id}/tables/${table.id}/clear`);
        setTables((current) => current.map((item) => item.id === table.id ? result.table : item));
        toast.success(`已结算并移出 ${result.settled_players} 名玩家，共 ${money(result.settled_cents)}`);
      } else {
        const force = table.player_count > 0 ? "?force=true" : "";
        await remove(`/api/spaces/${initialSpace.id}/tables/${table.id}${force}`);
        setTables((current) => current.filter((item) => item.id !== table.id));
        toast.success(table.player_count > 0 ? `已结算 ${table.player_count} 名玩家并删除牌桌` : "牌桌已删除");
      }
      setTableAdminAction(null);
      void loadLeaderboard();
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : mode === "clear" ? "清空牌桌失败" : "删除牌桌失败");
    } finally {
      setTableActionBusy(false);
    }
  }

  return (
    <div className="game-canvas channel-shell flex min-h-svh flex-col">
      <header className="game-topbar grid min-h-16 shrink-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 px-3 py-2 sm:px-6 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <div className="flex min-w-0 items-center gap-3">
          <Button variant="ghost" onClick={onBack}><ArrowLeft data-icon="inline-start" /><span className="hidden sm:inline">频道大厅</span></Button>
          <Separator orientation="vertical" className="h-8" />
          <BrandMark className="size-9 shrink-0" aria-hidden="true" />
          <div className="min-w-0"><h1 className="truncate text-sm font-semibold">{initialSpace.name}</h1><p className="truncate text-xs text-muted-foreground">{tables.length} 个牌桌 · {onlinePlayers} 人在线</p></div>
        </div>
        <nav className="order-3 col-span-2 flex items-center gap-1 justify-self-center md:order-none md:col-span-1" aria-label="游戏导航">
          <Button variant={gameType === "texas_holdem" ? "secondary" : "ghost"} onClick={() => setGameType("texas_holdem")}><Spade data-icon="inline-start" />德州扑克</Button>
          <Button variant={gameType === "landlord" ? "secondary" : "ghost"} onClick={() => setGameType("landlord")}><Crown data-icon="inline-start" />斗地主</Button>
        </nav>
        <div className="flex items-center justify-self-end gap-2"><Button size="sm" variant="outline" onClick={onOpenBindings}><Link2 data-icon="inline-start" /><span className="hidden sm:inline">频道账号</span></Button>{canManageBalances && <Button size="sm" variant="outline" onClick={onOpenBalances}><CircleDollarSign data-icon="inline-start" /><span className="hidden sm:inline">余额管理</span></Button>}{initialSpace.can_manage && initialSpace.invite_code && <Button size="sm" variant="outline" onClick={() => void copyInvite()}><Copy data-icon="inline-start" /><span className="hidden sm:inline">邀请码</span></Button>}</div>
      </header>

      <main className="min-w-0 flex-1 overflow-auto">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
            <Card className="min-w-0 [--card-spacing:--spacing(5)]">
              <CardHeader>
                <CardTitle>{gameType === "landlord" ? "斗地主" : "德州扑克"}牌局</CardTitle>
                <CardDescription>选择一张牌桌坐下，空位和玩家状态每 5 秒更新。</CardDescription>
                <CardAction className="flex gap-2"><Button variant="outline" disabled={!quickTable} onClick={() => quickTable && openTable(quickTable)}><Shuffle data-icon="inline-start" />{viewerTable ? "返回当前牌桌" : "快速加入"}</Button>{initialSpace.can_manage && <Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />新建牌桌</Button>}</CardAction>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
              {loading ? (
                  <div className="table-map-grid">{Array.from({ length: 6 }, (_, index) => <TableSkeleton key={index} />)}</div>
              ) : gameTables.length === 0 ? (
                  <Empty className="min-h-80 border"><EmptyHeader><EmptyMedia variant="icon"><Table2 /></EmptyMedia><EmptyTitle>还没有牌桌</EmptyTitle><EmptyDescription>{initialSpace.can_manage ? "创建第一张牌桌，频道就可以开局了。" : "频道管理员还没有创建牌桌。"}</EmptyDescription></EmptyHeader>{initialSpace.can_manage && <EmptyContent><Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />创建牌桌</Button></EmptyContent>}</Empty>
              ) : (
                  <div className="table-map-grid">{gameTables.map((table) => <TableMapTile key={table.id} table={table} blockedBySeat={!!viewerTable && !table.viewer_seated} onOpen={() => openTable(table)} onClear={initialSpace.can_manage && table.player_count > 0 ? () => setTableAdminAction({ mode: "clear", table }) : undefined} onDelete={initialSpace.can_manage ? () => setTableAdminAction({ mode: "delete", table }) : undefined} />)}{initialSpace.can_manage && <CreateTableTile onCreate={() => setCreateOpen(true)} />}</div>
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
        gameType={gameType}
        onClose={() => setCreateOpen(false)}
        onCreated={(table) => {
          setTables((current) => [table, ...current]);
          setCreateOpen(false);
          toast.success("牌桌已创建");
        }}
      />

      <AlertDialog open={tableAdminAction !== null} onOpenChange={(open) => !open && !tableActionBusy && setTableAdminAction(null)}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogMedia>{tableAdminAction?.mode === "clear" ? <Eraser /> : <Trash2 />}</AlertDialogMedia>
            <AlertDialogTitle>{tableAdminAction?.mode === "clear" ? `清空“${tableAdminAction.table.name}”？` : `删除“${tableAdminAction?.table.name}”？`}</AlertDialogTitle>
            <AlertDialogDescription>
              {tableAdminAction?.mode === "clear"
                ? `将依次结算并移出当前 ${tableAdminAction.table.player_count} 名玩家，牌桌配置会保留。`
                : tableAdminAction && tableAdminAction.table.player_count > 0
                  ? `将依次结算当前 ${tableAdminAction.table.player_count} 名玩家，再永久删除牌桌；已有资金流水仍会保留。`
                  : "牌桌及其牌局状态将永久删除，已有资金流水仍会保留。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {tableAdminAction && tableAdminAction.table.player_count > 0 && (
            <Alert variant="destructive">
              <AlertDescription>
                {tableHandActive(tableAdminAction.table.street)
                  ? "本手牌正在进行，必须等待本手结束后才能清空或强制删除。"
                  : "结算将逐人执行；若某笔余额回充失败，操作会停止，尚未结算的玩家仍保留在桌。"}
              </AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={tableActionBusy}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={tableActionBusy || !!(tableAdminAction && tableHandActive(tableAdminAction.table.street))} onClick={(event) => { event.preventDefault(); void runTableAdminAction(); }}>
              {tableActionBusy && <Spinner data-icon="inline-start" />}
              {tableActionBusy ? "正在处理…" : tableAdminAction?.mode === "clear" ? "结算并清空" : tableAdminAction?.table.player_count ? "结算并删除" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function TableMapTile({ table, blockedBySeat, onOpen, onClear, onDelete }: { table: TableSummary; blockedBySeat: boolean; onOpen: () => void; onClear?: () => void; onDelete?: () => void }) {
  const available = table.player_count < table.max_players;
  const players = [...(table.players || [])].sort((a, b) => a.seat - b.seat);
  const playerNames = players.map((player) => player.name).join("、");
  return (
    <div className="table-map-item group">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" className="table-map-tile" disabled={blockedBySeat} onClick={onOpen} aria-label={`${table.name}，${table.player_count}/${table.max_players} 人${playerNames ? `，玩家 ${playerNames}` : "，空桌"}${blockedBySeat ? "，你已在其他牌桌" : ""}`}>
            <span className="flex w-full min-w-0 items-center justify-between gap-2"><strong className="truncate text-sm">{tableDisplayName(table.name)}</strong><Badge className="shrink-0" variant={table.viewer_seated ? "default" : "outline"}>{table.player_count}/{table.max_players}</Badge></span>
            <span className="table-map-stage">
              <span className="table-map-surface"><strong>{table.game_type === "landlord" ? `底分 ${money(table.base_stake_cents || 0)}` : `${money(table.small_blind_cents || 0)}/${money(table.big_blind_cents || 0)}`}</strong><small>{available ? (table.player_count > 0 ? `${table.player_count} 人入座` : "等待入座") : "已满"}</small></span>
              {players.map((player, index) => <MapSeat key={player.user_id} player={player} style={mapSeatStyle(index, players.length)} />)}
            </span>
            <span className={cn("flex w-full items-center justify-between gap-2 text-xs text-muted-foreground", onClear ? "pr-14" : onDelete && "pr-7")}><span className="truncate">{table.game_type === "landlord" ? "斗地主" : "德州扑克"} · {tableHandActive(table.street) ? "进行中" : table.player_count > 0 ? "等待开局" : "等待玩家"}</span><span className="shrink-0">{table.viewer_seated ? "你在此桌" : blockedBySeat ? "已在其他桌" : "点击进入"}</span></span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{blockedBySeat ? "请先离开当前牌桌" : playerNames ? `${playerNames} 正在这桌` : "空桌，点击入座"}</TooltipContent>
      </Tooltip>
      {onClear && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="icon-xs" variant="ghost" className="absolute right-9 bottom-2 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100" onClick={onClear} aria-label={`清空牌桌 ${table.name}`}><Eraser /></Button>
          </TooltipTrigger>
          <TooltipContent side="top">结算全部玩家并清空牌桌</TooltipContent>
        </Tooltip>
      )}
      {onDelete && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="icon-xs" variant="ghost" className="absolute right-2 bottom-2 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100" onClick={onDelete} aria-label={`删除牌桌 ${table.name}`}><Trash2 /></Button>
          </TooltipTrigger>
          <TooltipContent side="top">{table.player_count > 0 ? "结算全部玩家并删除牌桌" : "删除牌桌"}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function tableHandActive(street: string) {
  return street === "preflop" || street === "flop" || street === "turn" || street === "river" || street === "bidding" || street === "playing";
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
  return <Button variant="ghost" className="table-map-tile items-center justify-center" onClick={onCreate}><Avatar size="lg"><AvatarFallback>+</AvatarFallback></Avatar><strong>创建新牌桌</strong><span className="text-xs text-muted-foreground">设置当前游戏的牌桌规则</span></Button>;
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
            <div className="flex w-0 min-w-full flex-col gap-1 pr-3">
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

function CreateTableDialog({ open, spaceID, gameType, onClose, onCreated }: { open: boolean; spaceID: string; gameType: GameType; onClose: () => void; onCreated: (table: TableSummary) => void }) {
  const [name, setName] = useState("");
  const [smallBlind, setSmallBlind] = useState("0.50");
  const [bigBlind, setBigBlind] = useState("1.00");
  const [baseStake, setBaseStake] = useState("1.00");
  const [actionTimeout, setActionTimeout] = useState("25");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const small = Math.round(Number(smallBlind) * 100);
    const big = Math.round(Number(bigBlind) * 100);
    const stake = Math.round(Number(baseStake) * 100);
    const timeout = Number(actionTimeout);
    if (gameType === "texas_holdem" && (!Number.isFinite(small) || !Number.isFinite(big) || small <= 0 || big < small)) {
      setError("请输入正确的大小盲金额");
      return;
    }
    if (gameType === "landlord" && (!Number.isFinite(stake) || stake <= 0 || stake > 100_000)) {
      setError("底分需在 $0.01–$1,000 之间");
      return;
    }
    if (!Number.isInteger(timeout) || timeout < 5 || timeout > 300) {
      setError("行动时限需为 5–300 秒之间的整数");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await post<{ table: TableSummary }>(`/api/spaces/${spaceID}/tables`, gameType === "landlord"
        ? { game_type: gameType, name, base_stake_cents: stake, action_timeout_seconds: timeout }
        : { game_type: gameType, name, small_blind_cents: small, big_blind_cents: big, action_timeout_seconds: timeout });
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
          <DialogHeader><DialogTitle>创建{gameType === "landlord" ? "斗地主" : "德州扑克"}牌桌</DialogTitle><DialogDescription>新牌桌使用当前频道的 New API 连接和成员体系。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <Field><FieldLabel htmlFor="table-name">牌桌名称</FieldLabel><Input id="table-name" name="table-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：休闲桌…" autoComplete="off" required autoFocus /></Field>
            {gameType === "texas_holdem" ? <div className="grid grid-cols-2 gap-4">
              <Field><FieldLabel htmlFor="small-blind">小盲（美元）</FieldLabel><Input id="small-blind" name="small-blind" type="number" min="0.01" step="0.01" value={smallBlind} onChange={(event) => setSmallBlind(event.target.value)} required /></Field>
              <Field><FieldLabel htmlFor="big-blind">大盲（美元）</FieldLabel><Input id="big-blind" name="big-blind" type="number" min="0.01" step="0.01" value={bigBlind} onChange={(event) => setBigBlind(event.target.value)} required /></Field>
            </div> : <Field><FieldLabel htmlFor="base-stake">底分（美元）</FieldLabel><Input id="base-stake" name="base-stake" type="number" min="0.01" max="1000" step="0.01" value={baseStake} onChange={(event) => setBaseStake(event.target.value)} required /><FieldDescription>每家输赢为底分 × 叫分 × 倍数，最多赔付桌上筹码。</FieldDescription></Field>}
            <Field data-invalid={error.startsWith("行动时限") || undefined}>
              <FieldLabel htmlFor="action-timeout">行动时限（秒）</FieldLabel>
              <Input id="action-timeout" name="action-timeout" type="number" min="5" max="300" step="1" inputMode="numeric" value={actionTimeout} onChange={(event) => setActionTimeout(event.target.value)} aria-invalid={error.startsWith("行动时限") || undefined} required />
              <FieldDescription><Clock3 />{gameType === "landlord" ? "超时自动不叫或不出；首家超时会自动出最小单牌。" : "每位玩家每次行动的倒计时；超时能过牌则自动过牌，否则自动弃牌。"}</FieldDescription>
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
