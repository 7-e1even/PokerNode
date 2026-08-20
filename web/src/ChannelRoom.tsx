import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  ArrowLeft, CircleDollarSign, Clock3, Copy, Crown, Eraser, Link2, MoreHorizontal, Plus, Spade, Table2, Trash2,
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
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { api, post, remove } from "./api";
import LandlordRoom from "./LandlordRoom";
import PokerRoom from "./PokerRoom";
import type { GameType, Space, TableSummary, User } from "./types";

interface Props {
  user: User;
  initialSpace: Space;
  initialTableID?: string;
  onBack: () => void;
  onNavigateTable: (tableID?: string) => void;
  onOpenBindings: () => void;
  onOpenBalances: () => void;
  onOpenHistory: () => void;
}

type TableAdminAction = { mode: "clear" | "delete"; table: TableSummary };

export default function ChannelRoom({ user, initialSpace, initialTableID, onBack, onNavigateTable, onOpenBindings, onOpenBalances, onOpenHistory }: Props) {
  const [space, setSpace] = useState(initialSpace);
  const [tables, setTables] = useState<TableSummary[]>([]);
  const [selectedTable, setSelectedTable] = useState<TableSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [spaceError, setSpaceError] = useState("");
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [gameType, setGameType] = useState<GameType>("texas_holdem");
  const [tableManagement, setTableManagement] = useState<TableSummary | null>(null);
  const [tableAdminAction, setTableAdminAction] = useState<TableAdminAction | null>(null);
  const [tableActionBusy, setTableActionBusy] = useState(false);
  const spaceRequestInFlight = useRef(false);
  const tablesRequestInFlight = useRef(false);

  const loadSpace = useCallback(async () => {
    if (spaceRequestInFlight.current) return;
    spaceRequestInFlight.current = true;
    try {
      const result = await api<{ space: Space }>(`/api/spaces/${initialSpace.id}`);
      setSpace(result.space);
      setSpaceError("");
    } catch (caught) {
      setSpaceError(caught instanceof Error ? caught.message : "读取频道信息失败");
    } finally {
      spaceRequestInFlight.current = false;
    }
  }, [initialSpace.id]);

  const loadTables = useCallback(async (showLoading = false) => {
    if (tablesRequestInFlight.current) return;
    tablesRequestInFlight.current = true;
    if (showLoading) setLoading(true);
    try {
      const result = await api<{ tables: TableSummary[] }>(`/api/spaces/${initialSpace.id}/tables`);
      setTables(result.tables || []);
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "读取牌桌失败");
    } finally {
      tablesRequestInFlight.current = false;
      if (showLoading) setLoading(false);
    }
  }, [initialSpace.id]);

  useEffect(() => { setSpace(initialSpace); }, [initialSpace]);

  useEffect(() => { void loadSpace(); }, [loadSpace]);

  useEffect(() => {
    void loadTables(true);
    const timer = window.setInterval(() => void loadTables(), 5_000);
    return () => window.clearInterval(timer);
  }, [loadTables]);

  useEffect(() => {
    if (!initialTableID) {
      setSelectedTable(null);
      return;
    }
    const target = tables.find((table) => table.id === initialTableID);
    if (target) {
      setGameType(target.game_type);
      setSelectedTable(target);
    } else if (!loading) {
      setSelectedTable(null);
      setError("该牌桌不存在或已不可用");
    }
  }, [initialTableID, loading, tables]);

  const gameTables = useMemo(() => tables.filter((table) => table.game_type === gameType), [gameType, tables]);
  const viewerTable = useMemo(() => tables.find((table) => table.viewer_seated), [tables]);
  const onlinePlayers = tables.reduce((total, table) => total + table.player_count, 0);
  const canManageBalances = space.is_owner || !!user.permissions?.includes("balances:manage");
  if (selectedTable) {
    if (selectedTable.game_type === "landlord") {
      return (
        <LandlordRoom
          user={user}
          initialSpace={space}
          initialTable={selectedTable}
          onBack={() => {
            setSelectedTable(null);
            onNavigateTable();
            void loadTables();
            void loadSpace();
          }}
        />
      );
    }
    return (
      <PokerRoom
        user={user}
        initialSpace={space}
        initialTable={selectedTable}
        onOpenHistory={onOpenHistory}
        onBack={() => {
          setSelectedTable(null);
          onNavigateTable();
          void loadTables();
          void loadSpace();
        }}
      />
    );
  }

  async function copyInvite() {
    try {
      await navigator.clipboard.writeText(space.invite_code || "");
      toast.success("频道邀请码已复制");
    } catch {
      toast.error("复制失败，请检查浏览器剪贴板权限");
    }
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
        const result = await post<{ settled_players: number; settled_cents: number; table: TableSummary }>(`/api/spaces/${space.id}/tables/${table.id}/clear`);
        setTables((current) => current.map((item) => item.id === table.id ? result.table : item));
        toast.success(`已结算并移出 ${result.settled_players} 名玩家，共 ${money(result.settled_cents)}`);
      } else {
        const force = table.player_count > 0 ? "?force=true" : "";
        await remove(`/api/spaces/${space.id}/tables/${table.id}${force}`);
        setTables((current) => current.filter((item) => item.id !== table.id));
        toast.success(table.player_count > 0 ? `已结算 ${table.player_count} 名玩家并删除牌桌` : "牌桌已删除");
      }
      setTableAdminAction(null);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : mode === "clear" ? "清空牌桌失败" : "删除牌桌失败");
    } finally {
      setTableActionBusy(false);
    }
  }

  const channelMain = (
    <main className="min-h-0 min-w-0 flex-1 overflow-auto bg-background">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <section className="flex min-w-0 flex-col gap-4" aria-labelledby="table-list-title">
          <header className="flex flex-col gap-1">
            <h2 id="table-list-title" className="font-heading text-xl font-semibold">{gameType === "landlord" ? "斗地主" : "德州扑克"}牌局</h2>
            <p className="text-sm text-muted-foreground">选择牌桌直接进入，空位和玩家状态每 5 秒更新。</p>
          </header>
          {(spaceError || error) && <Alert variant="destructive"><AlertDescription>{spaceError || error}</AlertDescription></Alert>}
          {loading ? (
            <div className="table-map-grid">{Array.from({ length: 6 }, (_, index) => <TableSkeleton key={index} />)}</div>
          ) : gameTables.length === 0 ? (
            <Empty className="min-h-80 border"><EmptyHeader><EmptyMedia variant="icon"><Table2 /></EmptyMedia><EmptyTitle>还没有牌桌</EmptyTitle><EmptyDescription>{space.can_manage ? "创建第一张牌桌，频道就可以开局了。" : "频道管理员还没有创建牌桌。"}</EmptyDescription></EmptyHeader>{space.can_manage && <EmptyContent><Button onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />创建牌桌</Button></EmptyContent>}</Empty>
          ) : (
            <div className="table-map-grid">
              {gameTables.map((table) => <TableMapTile key={table.id} table={table} blockedBySeat={!!viewerTable && !table.viewer_seated} onOpen={() => openTable(table)} onManage={space.can_manage ? () => setTableManagement(table) : undefined} />)}
              {space.can_manage && <CreateTableTile onCreate={() => setCreateOpen(true)} />}
            </div>
          )}
        </section>
      </div>
    </main>
  );

  return (
    <div className="game-canvas channel-shell flex h-svh flex-col overflow-hidden">
      <header className="game-topbar grid min-h-16 shrink-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 px-3 py-2 sm:px-6 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <div className="flex min-w-0 items-center gap-3">
          <Button className="min-h-11 min-w-11 lg:min-h-0 lg:min-w-0" variant="ghost" onClick={onBack} aria-label="返回频道大厅"><ArrowLeft data-icon="inline-start" /><span className="hidden sm:inline">频道大厅</span></Button>
          <Separator orientation="vertical" className="h-8" />
          <BrandMark className="size-9 shrink-0" aria-hidden="true" />
          <div className="min-w-0"><h1 className="truncate text-sm font-semibold">{space.name}</h1><p className="truncate text-xs text-muted-foreground">{tables.length} 个牌桌 · {onlinePlayers} 人在线</p></div>
        </div>
        <nav className="order-3 col-span-2 flex items-center gap-1 justify-self-center md:order-none md:col-span-1" aria-label="游戏导航">
          <Button className="min-h-11 lg:min-h-8" variant={gameType === "texas_holdem" ? "secondary" : "ghost"} onClick={() => setGameType("texas_holdem")}><Spade data-icon="inline-start" />德州扑克</Button>
          <Button className="min-h-11 lg:min-h-8" variant={gameType === "landlord" ? "secondary" : "ghost"} onClick={() => setGameType("landlord")}><Crown data-icon="inline-start" />斗地主</Button>
        </nav>
        <div className="flex items-center justify-self-end gap-2"><Button className="min-h-11 min-w-11 lg:min-h-0 lg:min-w-0" size="sm" variant="outline" onClick={onOpenBindings} aria-label="频道账号"><Link2 data-icon="inline-start" /><span className="hidden sm:inline">频道账号</span></Button>{canManageBalances && <Button className="min-h-11 min-w-11 lg:min-h-0 lg:min-w-0" size="sm" variant="outline" onClick={onOpenBalances} aria-label="余额管理"><CircleDollarSign data-icon="inline-start" /><span className="hidden sm:inline">余额管理</span></Button>}{space.can_manage && space.invite_code && <Button className="min-h-11 min-w-11 lg:min-h-0 lg:min-w-0" size="sm" variant="outline" onClick={() => void copyInvite()} aria-label="复制频道邀请码"><Copy data-icon="inline-start" /><span className="hidden sm:inline">邀请码</span></Button>}</div>
      </header>

      {channelMain}

      <CreateTableDialog
        open={createOpen}
        spaceID={space.id}
        gameType={gameType}
        onClose={() => setCreateOpen(false)}
        onCreated={(table) => {
          setTables((current) => [table, ...current]);
          setCreateOpen(false);
          toast.success("牌桌已创建");
        }}
      />

      <Dialog open={tableManagement !== null} onOpenChange={(open) => !open && setTableManagement(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>管理“{tableManagement?.name}”</DialogTitle>
            <DialogDescription>选择要如何处理这张牌桌。执行前还会再次确认。</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            {tableManagement && tableManagement.player_count > 0 && (
              <Button className="h-auto justify-start gap-3 px-3 py-3 text-left" variant="outline" onClick={() => { const table = tableManagement; setTableManagement(null); setTableAdminAction({ mode: "clear", table }); }}>
                <Eraser data-icon="inline-start" />
                <span className="flex min-w-0 flex-col items-start gap-0.5 whitespace-normal">
                  <strong>清空玩家，保留牌桌</strong>
                  <span className="text-xs font-normal text-muted-foreground">结算并移出当前 {tableManagement.player_count} 名玩家，牌桌配置保持不变。</span>
                </span>
              </Button>
            )}
            {tableManagement && (
              <Button className="h-auto justify-start gap-3 px-3 py-3 text-left" variant="destructive" onClick={() => { const table = tableManagement; setTableManagement(null); setTableAdminAction({ mode: "delete", table }); }}>
                <Trash2 data-icon="inline-start" />
                <span className="flex min-w-0 flex-col items-start gap-0.5 whitespace-normal">
                  <strong>永久删除牌桌</strong>
                  <span className="text-xs font-normal">{tableManagement.player_count > 0 ? `先结算并移出 ${tableManagement.player_count} 名玩家，再删除牌桌。` : "删除牌桌及当前牌局状态，资金流水仍会保留。"}</span>
                </span>
              </Button>
            )}
          </div>
          <DialogFooter><Button variant="ghost" onClick={() => setTableManagement(null)}>取消</Button></DialogFooter>
        </DialogContent>
      </Dialog>

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

function TableMapTile({ table, blockedBySeat, onOpen, onManage }: { table: TableSummary; blockedBySeat: boolean; onOpen: () => void; onManage?: () => void }) {
  const players = [...(table.players || [])].sort((a, b) => a.seat - b.seat);
  const playerNames = players.map((player) => player.name).join("、");
  const stakeLabel = table.game_type === "landlord" ? `底分 ${money(table.base_stake_cents || 0)}` : `${money(table.small_blind_cents || 0)} / ${money(table.big_blind_cents || 0)}`;
  const stateLabel = tableHandActive(table.street) ? "进行中" : table.player_count > 0 ? "等待开局" : "等待玩家";
  const manageable = !!onManage;
  const canWaitForNext = tableHandActive(table.street) && !table.viewer_seated && table.player_count < table.max_players;
  return (
    <div className="table-map-item group">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" className="table-map-tile" disabled={blockedBySeat} onClick={onOpen} aria-label={`${table.name}，${table.player_count}/${table.max_players} 人${playerNames ? `，玩家 ${playerNames}` : "，空桌"}${blockedBySeat ? "，你已在其他牌桌" : ""}`}>
            <span className={cn("flex w-full min-w-0 items-start justify-between gap-3", manageable && "pr-11")}>
              <strong className="min-w-0 truncate text-sm">{tableDisplayName(table.name)}</strong>
              <Badge className="shrink-0" variant={table.viewer_seated ? "default" : "outline"}>{table.player_count}/{table.max_players} 人</Badge>
            </span>
            <span className="flex w-full items-end justify-between gap-3">
              <span className="flex min-w-0 flex-col gap-1">
                <small className="text-xs font-normal text-muted-foreground">{table.game_type === "landlord" ? "本桌底分" : "小盲 / 大盲"}</small>
                <strong className="truncate text-base tabular-nums">{stakeLabel}</strong>
              </span>
              <Badge className="shrink-0" variant="secondary">{stateLabel}</Badge>
            </span>
            <span className="flex w-full items-center justify-between gap-3 border-t pt-3 text-xs font-normal text-muted-foreground">
              <span className="truncate">{playerNames ? `玩家：${playerNames}` : "暂无玩家"}</span>
              <span className="shrink-0">{table.viewer_seated ? "你在此桌" : blockedBySeat ? "已在其他桌" : canWaitForNext ? "可先入座" : "可进入"}</span>
            </span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{blockedBySeat ? "请先离开当前牌桌" : canWaitForNext ? "牌局进行中，可先入座等待下一手" : playerNames ? `${playerNames} 正在这桌` : "空桌，点击入座"}</TooltipContent>
      </Tooltip>
      {manageable && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="icon-sm" variant="ghost" className="absolute top-2 right-2 min-h-9 min-w-9 rounded-full opacity-100 transition-opacity lg:opacity-0 lg:group-hover:opacity-100 lg:focus-visible:opacity-100" onClick={onManage} aria-label={`管理牌桌 ${table.name}`}><MoreHorizontal /></Button>
          </TooltipTrigger>
          <TooltipContent side="top">管理牌桌</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function tableHandActive(street: string) {
  return street === "preflop" || street === "flop" || street === "turn" || street === "river" || street === "bidding" || street === "playing";
}

function CreateTableTile({ onCreate }: { onCreate: () => void }) {
  return <Button variant="ghost" className="table-map-tile items-center justify-center" onClick={onCreate}><Avatar size="lg"><AvatarFallback>+</AvatarFallback></Avatar><strong>创建新牌桌</strong><span className="text-xs text-muted-foreground">设置当前游戏的牌桌规则</span></Button>;
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
            <Field><FieldLabel htmlFor="table-name">牌桌名称</FieldLabel><Input id="table-name" name="table-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：休闲桌…" autoComplete="off" required /></Field>
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
  return <div className="table-map-tile pointer-events-none"><div className="flex items-center justify-between gap-3"><Skeleton className="h-8 w-24" /><Skeleton className="h-5 w-12" /></div><div className="flex items-end justify-between gap-3"><Skeleton className="h-10 w-28" /><Skeleton className="h-5 w-16" /></div><Skeleton className="h-4 w-full" /></div>;
}

function money(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2 }).format(cents / 100);
}

function tableDisplayName(name: string) {
  return name.replace(/\s*·\s*\$\d+(?:\.\d+)?\s*\/\s*\$\d+(?:\.\d+)?\s*$/, "");
}
