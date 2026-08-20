import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from "react";
import {
  ArrowLeft, Check, CircleDollarSign, CircleHelp, Clock3, Copy, Crown, DoorOpen, History,
  KeyRound, LogOut, MoreHorizontal, Server, Settings2, ShieldCheck, ThumbsDown, ThumbsUp, UserMinus,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogMedia, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Slider } from "@/components/ui/slider";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { api, post, put } from "./api";
import type { Balance, Card as PokerCard, KickVote, Membership, Player, Space, TableEnvelope, TableState, TableSummary, User, WalletOperation } from "./types";

interface Props {
  user: User;
  initialSpace: Space;
  initialTable: TableSummary;
  onBack: () => void;
}

interface ChipFlight {
  id: number;
  x: number;
  y: number;
  amount: number;
  delay: number;
}

interface PlayerLayout {
  player: Player;
  index: number;
  x: number;
  y: number;
}

const tableSeatCount = 8;

export default function PokerRoom({ user, initialSpace, initialTable, onBack }: Props) {
  const [space, setSpace] = useState(initialSpace);
  const [membership, setMembership] = useState<Membership | null>(null);
  const [table, setTable] = useState<TableState | null>(null);
  const [kickVote, setKickVote] = useState<KickVote | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [connection, setConnection] = useState<"connecting" | "live" | "polling" | "offline">("connecting");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [rulesOpen, setRulesOpen] = useState(false);
  const onBackRef = useRef(onBack);

  useEffect(() => { onBackRef.current = onBack; }, [onBack]);

  const reportError = useCallback((message: string) => toast.error(message), []);

  const loadBalance = useCallback(async () => {
    if (!space.is_bound) return;
    try {
      const result = await api<{ balance: Balance }>(`/api/spaces/${space.id}/balance`);
      setBalance(result.balance);
    } catch (caught) {
      reportError(caught instanceof Error ? caught.message : "余额读取失败");
    }
  }, [reportError, space.id, space.is_bound]);

  const loadRoom = useCallback(async () => {
    try {
      const [detail, tableResult] = await Promise.all([
        api<{ space: Space; membership: Membership }>(`/api/spaces/${space.id}`),
        api<TableEnvelope>(`/api/spaces/${space.id}/tables/${initialTable.id}`),
      ]);
      setSpace(detail.space);
      setMembership(detail.membership);
      setTable(tableResult.table);
      setKickVote(tableResult.kick_vote);
      if (detail.space.is_bound) void loadBalance();
    } catch (caught) {
      reportError(caught instanceof Error ? caught.message : "牌桌加载失败");
    }
  }, [initialTable.id, loadBalance, reportError, space.id]);

  useEffect(() => { void loadRoom(); }, [loadRoom]);

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const socketURL = `${protocol}//${window.location.host}/api/spaces/${space.id}/tables/${initialTable.id}/ws`;
    const tableURL = `/api/spaces/${space.id}/tables/${initialTable.id}`;
    let disposed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let pollTimer: number | undefined;
    let reconnectAttempts = 0;
    let pollInFlight = false;

    async function refreshTable() {
      if (disposed || pollInFlight) return;
      pollInFlight = true;
      try {
        const result = await api<TableEnvelope>(tableURL);
        if (!disposed) {
          setTable(result.table);
          setKickVote(result.kick_vote);
          if (socket?.readyState !== WebSocket.OPEN) setConnection("polling");
        }
      } catch {
        if (!disposed && socket?.readyState !== WebSocket.OPEN) setConnection("offline");
      } finally {
        pollInFlight = false;
      }
    }

    function stopPolling() {
      if (pollTimer !== undefined) window.clearInterval(pollTimer);
      pollTimer = undefined;
    }

    function startPolling() {
      if (disposed || pollTimer !== undefined) return;
      void refreshTable();
      pollTimer = window.setInterval(() => void refreshTable(), 2_000);
    }

    function scheduleReconnect() {
      if (disposed) return;
      startPolling();
      const delay = Math.min(1_000 * 2 ** reconnectAttempts, 15_000);
      reconnectAttempts += 1;
      reconnectTimer = window.setTimeout(connect, delay);
    }

    function connect() {
      if (disposed || socket?.readyState === WebSocket.CONNECTING || socket?.readyState === WebSocket.OPEN) return;
      setConnection(reconnectAttempts === 0 ? "connecting" : "polling");
      const nextSocket = new WebSocket(socketURL);
      socket = nextSocket;
      nextSocket.onopen = () => {
        if (disposed || socket !== nextSocket) return;
        reconnectAttempts = 0;
        stopPolling();
        setConnection("live");
        void refreshTable();
      };
      nextSocket.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          if (message.type === "table") {
            setTable(message.table);
            setKickVote(message.kick_vote ?? null);
          } else if (message.type === "table_deleted") {
            toast.info("牌桌已被管理员删除");
            onBackRef.current();
          }
        } catch { /* ignore malformed server messages */ }
      };
      nextSocket.onclose = () => {
        if (socket !== nextSocket) return;
        socket = null;
        scheduleReconnect();
      };
      nextSocket.onerror = () => nextSocket.close();
    }

    function reconnectNow() {
      if (disposed) return;
      reconnectAttempts = 0;
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      reconnectTimer = undefined;
      if (socket) socket.close();
      else connect();
    }

    connect();
    window.addEventListener("online", reconnectNow);
    return () => {
      disposed = true;
      window.removeEventListener("online", reconnectNow);
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      stopPolling();
      socket?.close();
    };
  }, [initialTable.id, space.id]);

  const previousViewerSeat = useRef<number | null>(null);
  useEffect(() => {
    if (!table) return;
    if (previousViewerSeat.current !== null && previousViewerSeat.current >= 0 && table.viewer_seat < 0) {
      void loadBalance();
    }
    previousViewerSeat.current = table.viewer_seat;
  }, [loadBalance, table]);

  return (
    <main className="poker-room flex h-svh min-h-0 flex-col overflow-hidden">
      <section className="poker-room__canvas relative min-h-0 flex-1 overflow-hidden">
        <div className="poker-room-hud pointer-events-none absolute inset-x-0 top-0 z-20 grid items-start gap-2 p-2 sm:p-3">
          <div className="poker-room-hud__identity pointer-events-auto flex min-w-0 items-center rounded-full border bg-background/90 p-1 pr-3 shadow-sm backdrop-blur-md">
            <IconButton label="返回频道" onClick={onBack}><ArrowLeft /></IconButton>
            <div className="min-w-0">
              <h1 className="truncate font-heading text-sm font-semibold">{table?.name || "No-Limit · $0.50 / $1"}</h1>
              <span className="hidden truncate text-xs text-muted-foreground md:block">
                {space.name} · {table ? `${money(table.small_blind_cents)} / ${money(table.big_blind_cents)}` : "6 人桌"}
              </span>
            </div>
          </div>

          {table && (
            <div className="poker-room-hud__status pointer-events-auto flex items-center gap-2 whitespace-nowrap rounded-full border bg-background/90 px-2 py-1 shadow-sm backdrop-blur-md" aria-live="polite">
              <Badge variant="secondary">第 {table.hand_id || 1} 手</Badge>
              <strong className="px-1 text-xs">{streetLabel(table.street)}</strong>
              <Separator orientation="vertical" className="h-4" />
              <span className="text-xs text-muted-foreground">盲注 {money(table.small_blind_cents)} / {money(table.big_blind_cents)}</span>
              {table.current_bet_cents > 0 && (
                <>
                  <Separator orientation="vertical" className="hidden h-4 sm:block" />
                  <span className="hidden text-xs text-muted-foreground sm:inline">{betStatusLabel(table)} · {money(table.current_bet_cents)}</span>
                </>
              )}
            </div>
          )}

          <div className="poker-room-hud__actions pointer-events-auto flex items-center gap-1 rounded-full border bg-background/90 p-1 shadow-sm backdrop-blur-md">
            <Badge variant={connection === "live" ? "secondary" : connection === "offline" ? "destructive" : "outline"} className="hidden lg:inline-flex" aria-live="polite">
              <span className={cn("size-1.5 rounded-full", connection === "live" ? "bg-foreground" : "bg-current")} aria-hidden="true" />
              {connection === "live" ? "实时" : connection === "polling" ? "自动同步" : connection === "connecting" ? "连接中" : "已断开"}
            </Badge>
            {space.is_bound && (
              <Button size="sm" variant="ghost" onClick={() => void loadBalance()} aria-label={balance ? `余额 ${money(balance.cents)}` : "刷新余额"}>
                <CircleDollarSign data-icon="inline-start" />
                <span className="hidden sm:inline">{balance ? money(balance.cents) : "—"}</span>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="icon" variant="ghost" aria-label="更多牌桌操作"><MoreHorizontal /></Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-44">
                <DropdownMenuGroup>
                  <DropdownMenuItem onSelect={() => setRulesOpen(true)}><CircleHelp />牌桌规则</DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => setHistoryOpen(true)}><History />资金记录</DropdownMenuItem>
                  {space.can_manage && <DropdownMenuItem onSelect={() => setSettingsOpen(true)}><Settings2 />频道设置</DropdownMenuItem>}
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  <DropdownMenuItem onSelect={onBack}><DoorOpen />离开牌桌</DropdownMenuItem>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>

        {table ? (
          <TableScene
            spaceID={space.id}
            tableID={initialTable.id}
            table={table}
            kickVote={kickVote}
            user={user}
            onError={reportError}
            onChanged={(nextTable, nextKickVote) => {
              setTable(nextTable);
              setKickVote(nextKickVote);
            }}
            onBalanceChanged={() => void loadBalance()}
          />
        ) : (
          <TableLoading />
        )}
        {!space.is_bound && membership && (
          <BindPanel
            space={space}
            onBound={(nextMembership, nextBalance) => {
              setMembership(nextMembership);
              setBalance(nextBalance);
              setSpace((current) => ({ ...current, is_bound: true }));
              toast.success("个人凭证已绑定");
            }}
          />
        )}
      </section>

      <SettingsDialog open={settingsOpen} space={space} onClose={() => setSettingsOpen(false)} onSaved={setSpace} />
      <HistorySheet open={historyOpen} space={space} onClose={() => setHistoryOpen(false)} />
      <RulesDialog open={rulesOpen} table={table} onClose={() => setRulesOpen(false)} />
    </main>
  );
}

function TableScene({ spaceID, tableID, table, kickVote, user, onError, onChanged, onBalanceChanged }: {
  spaceID: string;
  tableID: string;
  table: TableState;
  kickVote: KickVote | null;
  user: User;
  onError: (message: string) => void;
  onChanged: (table: TableState, kickVote: KickVote | null) => void;
  onBalanceChanged: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [buyIn, setBuyIn] = useState(10000);
  const [amount, setAmount] = useState(0);
  const [burnEvent, setBurnEvent] = useState(0);
  const [chipFlights, setChipFlights] = useState<ChipFlight[]>([]);
  const [kickTarget, setKickTarget] = useState<Player | null>(null);
  const [winnerStage, setWinnerStage] = useState<"announcement" | "crown" | null>(() => table.last_result ? "announcement" : null);
  const secondsRemaining = useActionCountdown(table.action_deadline_at, table.turn_id);
  const kickVoteSecondsRemaining = useActionCountdown(kickVote?.expires_at ?? 0, kickVote?.target_user_id ?? 0);
  const activeKickVote = kickVote && kickVoteSecondsRemaining !== 0 ? kickVote : null;
  const boardTracking = useRef({ handID: table.hand_id, length: table.board?.length || 0 });
  const potTracking = useRef({ handID: table.hand_id, total: totalPotForAnimation(table), actingSeat: table.acting_seat });
  const chipFlightID = useRef(0);
  const chipFlightTimers = useRef(new Set<number>());
  const seated = table.viewer_seat >= 0;
  const viewerPlayer = table.players.find((player) => player.user_id === user.id);
  const fundedPlayers = table.players.filter((player) => player.stack_cents > 0);
  const readyPlayers = fundedPlayers.filter((player) => player.ready);
  const allowed = table.allowed_actions;
  const actingPlayer = table.players.find((player) => player.seat === table.acting_seat);
  const resultHandID = table.last_result ? (table.last_result.hand_id ?? table.hand_id) : null;
  const winningUserIDs = useMemo(() => new Set(
    Object.entries(table.last_result?.payouts || {})
      .filter(([, payout]) => payout > 0)
      .map(([userID]) => Number(userID)),
  ), [table.last_result?.payouts]);
  const winnerNames = table.players.filter((player) => winningUserIDs.has(player.user_id)).map((player) => player.name).join("、");

  useEffect(() => {
    if (allowed.can_act) setAmount(allowed.min_raise_to_cents || Math.max(table.big_blind_cents, table.pot_cents));
  }, [allowed.can_act, allowed.min_raise_to_cents, table.big_blind_cents, table.hand_id, table.pot_cents]);

  useEffect(() => {
    const next = { handID: table.hand_id, length: table.board?.length || 0 };
    const previous = boardTracking.current;
    if (previous.handID === next.handID && next.length > previous.length) {
      setBurnEvent((event) => event + 1);
    }
    boardTracking.current = next;
  }, [table.board?.length, table.hand_id]);

  useEffect(() => {
    if (resultHandID === null) {
      setWinnerStage(null);
      return;
    }
    setWinnerStage("announcement");
    const timer = window.setTimeout(() => setWinnerStage("crown"), 2400);
    return () => window.clearTimeout(timer);
  }, [resultHandID]);

  useEffect(() => {
    const previous = potTracking.current;
    const nextTotal = totalPotForAnimation(table);
    const additions: ChipFlight[] = [];
    const layoutBySeat = new Map(playerLayouts(table).map((layout) => [layout.player.seat, layout]));

    if (previous.handID !== table.hand_id) {
      table.players
        .filter((player) => player.in_hand && player.bet_cents > 0)
        .sort((a, b) => a.seat - b.seat)
        .forEach((player, index) => {
          const layout = layoutBySeat.get(player.seat);
          if (!layout) return;
          additions.push({
            id: ++chipFlightID.current,
            x: layout.x - 50,
            y: layout.y - 50,
            amount: player.bet_cents,
            delay: index * 100,
          });
        });
    } else if (nextTotal > previous.total && previous.actingSeat >= 0) {
      const layout = layoutBySeat.get(previous.actingSeat);
      if (layout) {
        additions.push({
          id: ++chipFlightID.current,
          x: layout.x - 50,
          y: layout.y - 50,
          amount: nextTotal - previous.total,
          delay: 0,
        });
      }
    }

    potTracking.current = { handID: table.hand_id, total: nextTotal, actingSeat: table.acting_seat };
    if (additions.length === 0) return;

    const additionIDs = new Set(additions.map((flight) => flight.id));
    setChipFlights((current) => [...current, ...additions]);
    const timer = window.setTimeout(() => {
      setChipFlights((current) => current.filter((flight) => !additionIDs.has(flight.id)));
      chipFlightTimers.current.delete(timer);
    }, 1200);
    chipFlightTimers.current.add(timer);
  }, [table]);

  useEffect(() => () => {
    chipFlightTimers.current.forEach((timer) => window.clearTimeout(timer));
  }, []);

  async function run(path: string, body?: unknown, balanceChanged = false) {
    setBusy(true);
    try {
      const requestBody = path === "action" && body && typeof body === "object"
        ? { ...body, expected_turn_id: table.turn_id }
        : body;
      const result = await post<TableEnvelope>(`/api/spaces/${spaceID}/tables/${tableID}/${path}`, requestBody);
      if (result.table) onChanged(result.table, result.kick_vote ?? null);
      if (result.notice) toast.success(result.notice);
      if (balanceChanged) onBalanceChanged();
    } catch (caught) {
      onError(caught instanceof Error ? caught.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  const layouts = useMemo(() => playerLayouts(table), [table.players, table.viewer_seat]);

  return (
    <div className="poker-scene relative mx-auto h-full min-h-0 w-full max-w-360 overflow-hidden" data-actionable={allowed.can_act}>
      <div className="poker-table-layout absolute left-1/2 -translate-x-1/2 -translate-y-1/2">
        <div className="poker-table-surface absolute inset-0" />
        {burnEvent > 0 && handIsActive(table.street) && (
          <div className="poker-burn-card absolute" key={`${table.hand_id}-${burnEvent}`} aria-hidden="true"><BrandMark className="size-3" /></div>
        )}
        {chipFlights.map((flight) => (
          <div
            className="poker-chip-flight absolute"
            key={flight.id}
            style={{
              "--chip-delay": `${flight.delay}ms`,
              "--chip-x": `${flight.x}cqw`,
              "--chip-y": `${flight.y}cqh`,
            } as CSSProperties}
            aria-hidden="true"
          >
            <span className="poker-chip-token" />
            <Badge variant="secondary" className="poker-chip-amount tabular-nums">{money(flight.amount)}</Badge>
          </div>
        ))}

        <div className="poker-table-center absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
          {table.pot_cents > 0 && <Badge variant="outline" className="poker-pot absolute h-8 bg-background px-3 text-sm shadow-sm">底池&nbsp;<strong>{money(table.pot_cents)}</strong></Badge>}
          <div className="poker-community-cards flex">
            {Array.from({ length: 5 }, (_, index) => table.board?.[index]
              ? <PlayingCard key={`${table.hand_id}-${index}-${table.board[index].rank}-${table.board[index].suit}`} card={table.board[index]} dealIndex={boardDealIndex(table.street, index)} dealOrigin={boardDealOrigin(index)} motion="board" />
              : <PlayingCard key={`${table.hand_id}-${index}-slot`} placeholder />)}
          </div>
          {table.last_result && winnerStage !== "announcement" && (
            <div className="poker-hand-result absolute hidden items-center gap-2 rounded-full border bg-background px-3 py-1.5 text-xs shadow-sm sm:flex" aria-live="polite">
              <span className="text-muted-foreground">本手结束</span>
              <strong>{resultMessage(table.last_result.message)}</strong>
              <span className="font-medium tabular-nums">{money(table.last_result.pot_cents)}</span>
            </div>
          )}
        </div>

        {table.last_result && winnerStage === "announcement" && (
          <div className="poker-winner-announcement absolute top-1/2 left-1/2 z-20" key={`winner-${resultHandID}`} role="status" aria-live="assertive">
            <div className="poker-winner-announcement__panel flex flex-col items-center gap-2 rounded-3xl border bg-background px-8 py-6 text-center shadow-xl">
              <span className="poker-winner-announcement__crown grid size-11 place-items-center rounded-full bg-winner text-winner-foreground"><Crown aria-hidden="true" /></span>
              <span className="text-xs font-medium text-muted-foreground">本手赢家</span>
              <strong className="max-w-64 text-balance font-heading text-2xl">{winnerNames || resultMessage(table.last_result.message)}</strong>
              <span className="text-sm text-muted-foreground">赢得 <strong className="text-foreground tabular-nums">{money(table.last_result.pot_cents)}</strong></span>
            </div>
          </div>
        )}

        {layouts.map((layout) => (
          <Seat
            key={layout.player.user_id}
            layout={layout}
            isViewer={layout.player.user_id === user.id}
            table={table}
            isWinner={winningUserIDs.has(layout.player.user_id)}
            showWinnerCrown={winnerStage === "crown"}
            canRequestKick={seated && table.can_start && layout.player.user_id !== user.id && layout.player.stack_cents > 0 && !layout.player.ready && !activeKickVote}
            onRequestKick={() => setKickTarget(layout.player)}
          />
        ))}
      </div>

      {!seated && (
        <ActionCard className="w-[min(34rem,calc(100%-2rem))]">
          <div className="flex items-center justify-between"><span className="text-sm text-muted-foreground">坐下买入</span><strong>{money(buyIn)}</strong></div>
          <Slider aria-label="买入金额" min={2000} max={100000} step={500} value={[buyIn]} onValueChange={(value) => setBuyIn(value[0])} />
          <Button className="rounded-full" size="lg" disabled={busy} onClick={() => void run("join", { buy_in_cents: buyIn }, true)}>
            {busy && <Spinner data-icon="inline-start" />}{busy ? "处理中…" : "加入牌桌"}
          </Button>
        </ActionCard>
      )}

      {seated && !allowed.can_act && (
        <ActionCard className={table.can_start ? "w-[min(38rem,calc(100%-2rem))]" : "w-auto"}>
          <div className="flex flex-col items-stretch justify-between gap-3 sm:flex-row sm:items-center sm:gap-4">
            {activeKickVote ? (
              <>
                <div className="min-w-0 flex-1">
                  <strong className="block truncate text-sm">是否移出 {activeKickVote.target_name}？</strong>
                  <span className="block truncate text-xs text-muted-foreground">
                    {activeKickVote.initiator_name} 发起 · 同意 {activeKickVote.yes_count}/{activeKickVote.required_yes} · 剩余 {kickVoteSecondsRemaining ?? 0} 秒
                  </span>
                </div>
                <div className="flex shrink-0 items-center justify-end gap-1">
                  {activeKickVote.target_user_id === user.id ? (
                    <>
                      {table.can_leave && <Button size="sm" variant="ghost" disabled={busy} onClick={() => void run("leave", {}, true)}><LogOut data-icon="inline-start" />结算离桌</Button>}
                      <Button className="rounded-full" size="lg" disabled={busy || viewerPlayer?.ready} onClick={() => void run("ready")}>
                        {busy ? <Spinner data-icon="inline-start" /> : <Check data-icon="inline-start" />}
                        {busy ? "准备中…" : "我在，立即准备"}
                      </Button>
                    </>
                  ) : activeKickVote.can_vote ? (
                    <>
                      <Button size="sm" variant="outline" disabled={busy} onClick={() => void run("kick-vote", { action: "reject" })}><ThumbsDown data-icon="inline-start" />反对</Button>
                      <Button className="rounded-full" size="lg" variant="destructive" disabled={busy} onClick={() => void run("kick-vote", { action: "approve" })}>
                        {busy ? <Spinner data-icon="inline-start" /> : <ThumbsUp data-icon="inline-start" />}{busy ? "提交中…" : "同意移出"}
                      </Button>
                    </>
                  ) : (
                    <Badge variant="outline">{activeKickVote.viewer_vote === "approve" ? "你已同意" : activeKickVote.viewer_vote === "reject" ? "你已反对" : "等待表决"}</Badge>
                  )}
                </div>
              </>
            ) : table.can_start ? (
              <>
                <div className="min-w-0">
                  <strong className="block truncate text-sm">
                    <span className="sm:hidden">{table.last_result ? `${resultMessage(table.last_result.message)} · ${money(table.last_result.pot_cents)}` : `准备第 ${table.hand_id + 1} 手`}</span>
                    <span className="hidden sm:inline">准备第 {table.hand_id + 1} 手</span>
                  </strong>
                  <span className="block truncate text-xs text-muted-foreground"><span className="sm:hidden">第 {table.hand_id + 1} 手 · </span>已准备 {readyPlayers.length}/{fundedPlayers.length}，全员准备后自动发牌</span>
                </div>
                <div className="flex shrink-0 items-center justify-end gap-1">
                  {table.can_leave && <Button size="sm" variant="ghost" disabled={busy} onClick={() => void run("leave", {}, true)}><LogOut data-icon="inline-start" />结算离桌</Button>}
                  <Button className="rounded-full" size="lg" disabled={busy || viewerPlayer?.ready} onClick={() => void run("ready")}>
                    {busy ? <Spinner data-icon="inline-start" /> : <Check data-icon="inline-start" />}
                    {busy ? "准备中…" : viewerPlayer?.ready ? "已准备" : "准备"}
                  </Button>
                </div>
              </>
            ) : (
              <span className="flex items-center gap-2 whitespace-nowrap px-2 text-sm text-muted-foreground"><Clock3 aria-hidden="true" />{table.acting_seat >= 0 ? `等待 ${actingPlayer?.name || "其他玩家"} 行动 · ${secondsRemaining ?? table.action_timeout_seconds} 秒` : "等待更多有筹码的玩家入座"}</span>
            )}
            {!table.can_start && table.can_leave && <Button className="rounded-full" size="lg" variant="outline" disabled={busy} onClick={() => void run("leave", {}, true)}><LogOut data-icon="inline-start" />结算离桌</Button>}
          </div>
        </ActionCard>
      )}

      {seated && allowed.can_act && (
        <div className="poker-action-region absolute left-1/2 z-10 flex w-[min(46rem,calc(100%-2rem))] -translate-x-1/2 flex-col items-center gap-2">
          {(allowed.can_bet || allowed.can_raise) && (
            <Card size="sm" className="w-full rounded-2xl py-0 shadow-md sm:rounded-full">
              <CardContent className="poker-bet-controls flex flex-wrap items-center gap-2 p-2.5 sm:gap-3">
                <div className="poker-bet-heading flex w-full flex-col px-1 sm:min-w-28 sm:w-auto">
                  <strong className="text-xs">轮到你行动</strong>
                  <span className="text-[0.6875rem] text-muted-foreground">最小至 {money(allowed.min_raise_to_cents)}</span>
                </div>
                <ToggleGroup className="poker-quick-bets shrink-0" type="single" variant="outline" size="sm" aria-label="快捷下注比例" onValueChange={(value) => value && setAmount(quickAmount(table, Number(value)))}>
                  {quickBets.map(({ label, ratio }) => (
                    <ToggleGroupItem className="rounded-full" key={label} value={String(ratio)}>{label}</ToggleGroupItem>
                  ))}
                </ToggleGroup>
                <Separator orientation="vertical" className="hidden h-6 sm:block" />
                <Slider aria-label={allowed.can_bet ? "下注金额" : "加注金额"} className="poker-bet-slider order-last w-full sm:order-none sm:min-w-24 sm:flex-1" min={allowed.min_raise_to_cents} max={allowed.max_raise_to_cents} step={50} value={[clamp(amount, allowed.min_raise_to_cents, allowed.max_raise_to_cents)]} onValueChange={(value) => setAmount(value[0])} />
                <strong className="poker-bet-amount min-w-22 text-right">{money(amount)}</strong>
              </CardContent>
            </Card>
          )}
          <Card size="sm" className="w-fit max-w-full rounded-2xl py-0 shadow-lg sm:rounded-full">
            <CardContent className="poker-action-buttons flex flex-wrap items-center justify-center gap-2 p-2">
              <Badge variant={(secondsRemaining ?? table.action_timeout_seconds) <= 5 ? "destructive" : "outline"} className="poker-turn-timer h-9 shrink-0 rounded-full px-3 tabular-nums"><Clock3 aria-hidden="true" />剩余 {secondsRemaining ?? table.action_timeout_seconds} 秒</Badge>
              {allowed.can_fold && <Button className="min-w-22 rounded-full" size="lg" variant="outline" disabled={busy} onClick={() => void run("action", { action: "fold", amount_cents: 0 })}>弃牌</Button>}
              {allowed.can_check && <Button className="min-w-22 rounded-full" size="lg" variant="outline" disabled={busy} onClick={() => void run("action", { action: "check", amount_cents: 0 })}>过牌</Button>}
              {allowed.can_call && <Button className="min-w-30 rounded-full" size="lg" variant="outline" disabled={busy} onClick={() => void run("action", { action: "call", amount_cents: 0 })}>跟注 {money(allowed.to_call_cents)}</Button>}
              {allowed.can_all_in && <Button className="min-w-22 rounded-full" size="lg" variant="secondary" disabled={busy} onClick={() => void run("action", { action: "all_in", amount_cents: 0 })}>全下</Button>}
              {(allowed.can_bet || allowed.can_raise) && <Button className="min-w-36 rounded-full" size="lg" disabled={busy} onClick={() => void run("action", { action: allowed.can_bet ? "bet" : "raise", amount_cents: amount })}>{aggressiveActionLabel(table)} {money(amount)}</Button>}
            </CardContent>
          </Card>
        </div>
      )}

      <AlertDialog open={kickTarget !== null} onOpenChange={(open) => !open && setKickTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia><UserMinus /></AlertDialogMedia>
            <AlertDialogTitle>发起移出 {kickTarget?.name} 的投票？</AlertDialogTitle>
            <AlertDialogDescription>
              发起者自动计一票，除该玩家外的在桌玩家严格过半同意即通过。通过后会自动结算其筹码，并在 5 分钟内禁止重新入座。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={busy} onClick={() => kickTarget && void run("kick-vote", { action: "start", target_user_id: kickTarget.user_id })}>
              <UserMinus data-icon="inline-start" />发起投票
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ActionCard({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <Card size="sm" className={cn("poker-action-pod absolute left-1/2 z-10 -translate-x-1/2 gap-0 rounded-2xl py-0", className)}>
      <CardContent className="grid gap-3 p-4">{children}</CardContent>
    </Card>
  );
}

function Seat({ layout, isViewer, table, isWinner, showWinnerCrown, canRequestKick, onRequestKick }: {
  layout: PlayerLayout;
  isViewer: boolean;
  table: TableState;
  isWinner: boolean;
  showWinnerCrown: boolean;
  canRequestKick: boolean;
  onRequestKick: () => void;
}) {
  const { player, x, y } = layout;
  const positions = seatPositions(table, player.seat);
  const action = player.is_acting ? "轮到行动" : playerActionLabel(player);
  const dealOrigin = { x: `${roundLayout(76.5 - x)}cqw`, y: `${roundLayout(50 - y)}cqh` };
  return (
    <div
      className={cn("poker-seat absolute z-10 -translate-x-1/2 -translate-y-1/2", player.folded && "opacity-45")}
      data-seat={player.seat}
      data-visual-index={layout.index}
      data-horizontal-edge={x <= 15 || x >= 85 ? "true" : undefined}
      style={{ "--poker-seat-x": `${x}%`, "--poker-seat-y": `${y}%` } as CSSProperties}
    >
      {player.cards && player.cards.length > 0 && <div className="poker-hole-cards absolute left-1/2 flex -translate-x-1/2">{player.cards.map((card, index) => <PlayingCard card={card} dealIndex={holeCardDealIndex(table, player.seat, index)} dealOrigin={dealOrigin} motion="hole" key={`${table.hand_id}-${index}-${card.rank}-${card.suit}`} />)}</div>}
      {!player.cards && player.in_hand && <div className="poker-hole-cards absolute left-1/2 flex -translate-x-1/2"><PlayingCard hidden dealIndex={holeCardDealIndex(table, player.seat, 0)} dealOrigin={dealOrigin} motion="hole" key={`${table.hand_id}-back-0`} /><PlayingCard hidden dealIndex={holeCardDealIndex(table, player.seat, 1)} dealOrigin={dealOrigin} motion="hole" key={`${table.hand_id}-back-1`} /></div>}
      <Card size="sm" className="poker-seat-card relative w-38 gap-0 overflow-visible rounded-full bg-background py-2 shadow-sm md:w-max md:min-w-42 md:max-w-54" data-acting={player.is_acting ? "true" : undefined}>
        {isWinner && showWinnerCrown && <span className="poker-winner-crown absolute -top-4 left-1/2 grid size-7 -translate-x-1/2 place-items-center rounded-full bg-winner text-winner-foreground shadow-md" role="img" aria-label="本手赢家"><Crown aria-hidden="true" /></span>}
        <CardContent className="flex items-center gap-1.5 px-2 md:gap-2">
          <Avatar className="size-8 md:size-10">
            <AvatarFallback>{player.name.slice(0, 2).toUpperCase()}</AvatarFallback>
          </Avatar>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-1">
              <strong className="min-w-0 flex-1 truncate text-sm">{player.name}{isViewer && <span className="hidden sm:inline">（你）</span>}</strong>
              {positions.length > 0 && (
                <Badge variant={positions.some((position) => position.code === "BTN") ? "default" : "outline"} className="h-4 shrink-0 px-1 text-[0.5rem] md:h-5 md:px-1.5 md:text-[0.625rem]" aria-label={positions.map((position) => position.label).join("、")}>
                  {positions.map((position) => position.code).join("/")}
                </Badge>
              )}
            </div>
            <span className="block text-xs text-muted-foreground tabular-nums">{money(player.stack_cents)}</span>
          </div>
          {canRequestKick && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="icon-xs" variant="ghost" aria-label={`投票移出 ${player.name}`} onClick={onRequestKick}><UserMinus /></Button>
              </TooltipTrigger>
              <TooltipContent>投票移出未准备玩家</TooltipContent>
            </Tooltip>
          )}
        </CardContent>
      </Card>
      {(action || player.bet_cents > 0) && (
        <Badge variant={player.is_acting ? "outline" : "secondary"} className={cn("poker-seat-action absolute max-w-40 truncate bg-background shadow-sm", seatBetClass(x, y))}>
          {action}{action && player.bet_cents > 0 ? " · " : ""}{player.bet_cents > 0 ? money(player.bet_cents) : ""}
        </Badge>
      )}
    </div>
  );
}

function PlayingCard({ card, hidden = false, placeholder = false, dealIndex = 0, dealOrigin, motion }: {
  card?: PokerCard;
  hidden?: boolean;
  placeholder?: boolean;
  dealIndex?: number;
  dealOrigin?: { x: string; y: string };
  motion?: "hole" | "board";
}) {
  if (placeholder) {
    return <div className="poker-card-slot h-22 w-[3.875rem] rounded-lg border" aria-hidden="true" />;
  }
  const style = {
    "--deal-index": dealIndex,
    "--deal-x": dealOrigin?.x || "0rem",
    "--deal-y": dealOrigin?.y || "0rem",
  } as CSSProperties;
  if (hidden || !card) {
    return (
      <div className="poker-card-shell" data-motion={motion} style={style} role="img" aria-label="牌背">
        <div className="poker-card-turn relative h-22 w-[3.875rem]">
          <div className="poker-playing-card poker-playing-card--back absolute inset-0 grid place-items-center rounded-lg border bg-card text-muted-foreground shadow-sm"><BrandMark className="size-6" aria-hidden="true" /></div>
        </div>
      </div>
    );
  }
  const ranks: Record<number, string> = { 10: "10", 11: "J", 12: "Q", 13: "K", 14: "A" };
  const suits = ["♣", "♦", "♥", "♠"];
  const red = card.suit === 1 || card.suit === 2;
  const rank = ranks[card.rank] || card.rank;
  return (
    <div className="poker-card-shell" data-motion={motion} data-reveal="true" style={style} role="img" aria-label={`${rank}${suits[card.suit]}`}>
      <div className="poker-card-turn relative h-22 w-[3.875rem]">
        <div className={cn("poker-playing-card poker-playing-card--face absolute inset-0 flex flex-col rounded-lg border bg-card p-2 font-heading text-xl font-semibold shadow-sm", red && "text-destructive")}>
          <strong className="leading-none">{rank}</strong><span className="leading-none">{suits[card.suit]}</span>
        </div>
        <div className="poker-playing-card poker-playing-card--back poker-playing-card--flip-back absolute inset-0 grid place-items-center rounded-lg border bg-card text-muted-foreground shadow-sm" aria-hidden="true">
          <BrandMark className="size-6" />
        </div>
      </div>
    </div>
  );
}

export function BindPanel({ space, onBound }: { space: Space; onBound: (member: Membership, balance: Balance) => void }) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await post<{ membership: Membership; balance: Balance }>(`/api/spaces/${space.id}/bind`, { token });
      onBound(result.membership, result.balance);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "绑定失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open>
      <DialogContent showCloseButton={false} onPointerDownOutside={(event) => event.preventDefault()} onEscapeKeyDown={(event) => event.preventDefault()}>
        <form onSubmit={submit}>
          <DialogHeader>
            <div className="mb-2 grid size-10 place-items-center rounded-xl bg-muted"><KeyRound /></div>
            <DialogTitle>绑定当前频道的个人凭证</DialogTitle>
            <DialogDescription>
              使用 {hostOf(space.newapi_base_url)} 的 System Access Token。它只标识你在当前频道的 New API 账户，不等于 PokerNode 登录密码。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup className="mt-6">
            <Field>
              <FieldLabel htmlFor="personal-token">个人 System Access Token</FieldLabel>
              <Input id="personal-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="粘贴你的 System Access Token" required autoFocus />
              <FieldDescription className="flex items-center gap-1.5"><ShieldCheck />AES-256-GCM 加密保存，界面仅展示末四位</FieldDescription>
              <FieldError>{error}</FieldError>
            </Field>
          </FieldGroup>
          <DialogFooter className="mt-6">
            <Button className="w-full" size="lg" disabled={busy}>
              {busy ? <Spinner data-icon="inline-start" /> : <Check data-icon="inline-start" />}{busy ? "正在验证…" : "验证并绑定"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function SettingsDialog({ open, space, onClose, onSaved }: { open: boolean; space: Space; onClose: () => void; onSaved: (space: Space) => void }) {
  const [baseURL, setBaseURL] = useState(space.newapi_base_url);
  const [token, setToken] = useState("");
  const [quota, setQuota] = useState(space.quota_per_usd);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setBaseURL(space.newapi_base_url);
      setToken("");
      setQuota(space.quota_per_usd);
      setError("");
    }
  }, [open, space]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await put<{ space: Space }>(`/api/spaces/${space.id}/connection`, { newapi_base_url: baseURL, admin_token: token, quota_per_usd: quota });
      onSaved(result.space);
      onClose();
      toast.success("频道设置已保存");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function copyInvite() {
    await navigator.clipboard.writeText(space.invite_code || "");
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>频道设置</DialogTitle>
            <DialogDescription>修改连接时需要重新输入管理员凭证并在线验证。</DialogDescription>
          </DialogHeader>
          <FieldGroup className="mt-6">
            <Field>
              <FieldLabel>成员邀请码</FieldLabel>
              <div className="flex gap-2">
                <Input readOnly value={space.invite_code || ""} className="font-mono" />
                <Button type="button" variant="outline" onClick={() => void copyInvite()}>{copied ? <Check data-icon="inline-start" /> : <Copy data-icon="inline-start" />}{copied ? "已复制" : "复制"}</Button>
              </div>
            </Field>
            <Field>
              <FieldLabel htmlFor="settings-url"><Server />New API 地址</FieldLabel>
              <Input id="settings-url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} required />
            </Field>
            <Field>
              <FieldLabel htmlFor="settings-token"><KeyRound />新管理员凭证</FieldLabel>
              <Input id="settings-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder={`当前 ····${space.admin_token_last4}`} required />
            </Field>
            <Field>
              <FieldLabel htmlFor="settings-quota">每美元对应 quota</FieldLabel>
              <Input id="settings-quota" type="number" min={100} step={100} value={quota} onChange={(event) => setQuota(Number(event.target.value))} required />
              <FieldDescription>默认 500,000 quota = $1.00。PokerNode 内部始终按整数美分记账。</FieldDescription>
            </Field>
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6">
            <Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在验证…" : "验证并保存"}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function RulesDialog({ open, table, onClose }: { open: boolean; table: TableState | null; onClose: () => void }) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>牌桌规则</DialogTitle>
          <DialogDescription>当前牌桌采用无限注德州扑克规则，界面会按实际下注层级标记 3-bet、4-bet 及后续再加注。</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-5 text-sm">
          <section className="flex flex-col gap-3" aria-labelledby="rules-table">
            <div className="flex items-center justify-between gap-3">
              <h3 id="rules-table" className="font-semibold">牌局结构</h3>
              <Badge variant="secondary">No-Limit Hold’em · 8-max</Badge>
            </div>
            <dl className="grid grid-cols-[6rem_1fr] gap-x-4 gap-y-2 text-muted-foreground">
              <dt>盲注</dt><dd className="text-foreground">{table ? `${money(table.small_blind_cents)} / ${money(table.big_blind_cents)}` : "—"}</dd>
              <dt>位置</dt><dd><strong className="text-foreground">BTN</strong> 庄位，<strong className="text-foreground">SB</strong> 小盲，<strong className="text-foreground">BB</strong> 大盲；每手顺时针轮转。</dd>
              <dt>行动顺序</dt><dd>翻牌前从大盲左侧开始；翻牌后从庄位左侧仍在牌局的玩家开始。</dd>
              <dt>行动时限</dt><dd>每次行动 {table ? `${table.action_timeout_seconds} 秒` : "—"}；超时能过牌则自动过牌，否则自动弃牌。</dd>
            </dl>
          </section>
          <Separator />
          <section className="flex flex-col gap-3" aria-labelledby="rules-betting">
            <h3 id="rules-betting" className="font-semibold">下注与再加注</h3>
            <dl className="grid grid-cols-[6rem_1fr] gap-x-4 gap-y-2 text-muted-foreground">
              <dt>3-bet / 4-bet</dt><dd>翻牌前大盲是第 1 注，首次加注是 2-bet，再次加注为 3-bet，后续依次递增。</dd>
              <dt>最小加注</dt><dd>加注幅度不得小于上一笔完整加注；操作区会直接显示“最小至”金额。</dd>
              <dt>短码 All-in</dt><dd>不足一个完整加注的 All-in 不会重新开放已经行动玩家的加注权。</dd>
            </dl>
          </section>
          <Separator />
          <section className="flex flex-col gap-2" aria-labelledby="rules-next-hand">
            <h3 id="rules-next-hand" className="font-semibold">准备下一手</h3>
            <p className="text-muted-foreground">本手结算后，每位有筹码的在桌玩家都需要点击“准备”；全员准备后自动发牌，庄位和盲注会自动轮转。</p>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function HistorySheet({ open, space, onClose }: { open: boolean; space: Space; onClose: () => void }) {
  const [operations, setOperations] = useState<WalletOperation[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError("");
    api<{ operations: WalletOperation[] }>(`/api/spaces/${space.id}/operations`)
      .then((result) => setOperations(result.operations || []))
      .catch((caught) => setError(caught instanceof Error ? caught.message : "读取记录失败"))
      .finally(() => setLoading(false));
  }, [open, space.id]);

  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent className="sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>资金记录</SheetTitle>
          <SheetDescription>最近 50 笔买入和离桌结算。异常操作不会自动重试。</SheetDescription>
        </SheetHeader>
        <Separator />
        <ScrollArea className="min-h-0 flex-1 px-4">
          {error ? (
            <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>
          ) : loading ? (
            <div className="grid gap-3">{Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-12 w-full" />)}</div>
          ) : operations.length === 0 ? (
            <Empty className="min-h-72">
              <EmptyHeader>
                <EmptyMedia variant="icon"><CircleDollarSign /></EmptyMedia>
                <EmptyTitle>还没有资金记录</EmptyTitle>
                <EmptyDescription>完成一次买入或离桌结算后，记录会显示在这里。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table>
              <TableHeader><TableRow><TableHead>类型</TableHead><TableHead>牌桌</TableHead><TableHead>时间</TableHead><TableHead>金额</TableHead><TableHead>状态</TableHead></TableRow></TableHeader>
              <TableBody>
                {operations.map((operation) => (
                  <TableRow key={operation.id}>
                    <TableCell className="font-medium">{operation.kind === "buy_in" ? "买入牌桌" : "离桌结算"}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{operation.table_id === "main" ? "默认桌" : operation.table_id.slice(0, 8)}</TableCell>
                    <TableCell className="text-muted-foreground">{new Date(operation.created_at).toLocaleString()}</TableCell>
                    <TableCell className="font-mono">{operation.kind === "buy_in" ? "−" : "+"}{money(operation.cents)}</TableCell>
                    <TableCell><Badge variant={operation.status === "completed" ? "secondary" : operation.status === "manual_review" ? "destructive" : "outline"}>{statusLabel(operation.status)}</Badge></TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ScrollArea>
      </SheetContent>
    </Sheet>
  );
}

function IconButton({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild><Button size="icon" variant="ghost" onClick={onClick} aria-label={label}>{children}</Button></TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function TableLoading() {
  return (
    <div className="mx-auto grid h-full min-h-0 min-w-0 place-items-center">
      <div className="flex items-center gap-3 text-sm text-muted-foreground"><Spinner />正在布置牌桌…</div>
    </div>
  );
}

const quickBets = [
  { label: "1/3", ratio: 1 / 3 },
  { label: "1/2", ratio: 1 / 2 },
  { label: "3/4", ratio: 3 / 4 },
  { label: "底池", ratio: 1 },
];

function handIsActive(street: string) {
  return street === "preflop" || street === "flop" || street === "turn" || street === "river";
}

function useActionCountdown(deadlineAt: number, turnID: number) {
  const [remaining, setRemaining] = useState<number | null>(() => secondsUntil(deadlineAt));

  useEffect(() => {
    const update = () => setRemaining(secondsUntil(deadlineAt));
    update();
    if (deadlineAt <= 0) return;
    const timer = window.setInterval(update, 250);
    return () => window.clearInterval(timer);
  }, [deadlineAt, turnID]);

  return remaining;
}

function secondsUntil(deadlineAt: number) {
  return deadlineAt > 0 ? Math.max(0, Math.ceil((deadlineAt - Date.now()) / 1000)) : null;
}

function boardDealIndex(street: string, cardIndex: number) {
  if (street === "turn" && cardIndex === 3) return 0;
  if (street === "river" && cardIndex === 4) return 0;
  return cardIndex;
}

function boardDealOrigin(cardIndex: number) {
  const offset = (cardIndex - 2) * 4.625;
  if (offset === 0) return { x: "26.5cqw", y: "0rem" };
  return { x: `calc(26.5cqw ${offset < 0 ? "+" : "-"} ${Math.abs(offset)}rem)`, y: "0rem" };
}

function playerLayouts(table: TableState): PlayerLayout[] {
  let players = [...(table.players || [])].sort((a, b) => a.seat - b.seat);
  const viewerIndex = table.viewer_seat >= 0 ? players.findIndex((player) => player.seat === table.viewer_seat) : -1;
  if (viewerIndex > 0) players = [...players.slice(viewerIndex), ...players.slice(0, viewerIndex)];
  if (players.length === 0) return [];

  return players.map((player, index) => {
    const angle = (90 + index * 360 / players.length) * Math.PI / 180;
    return {
      player,
      index,
      x: roundLayout(50 + 44 * Math.cos(angle)),
      y: roundLayout(50 + 50 * Math.sin(angle)),
    };
  });
}

function roundLayout(value: number) {
  return Math.round(value * 1000) / 1000;
}

function seatBetClass(x: number, y: number) {
  if (y <= 30) return "poker-seat-action--below-seat";
  if (y >= 70) return "poker-seat-action--above-cards left-1/2 -translate-x-1/2";
  if (x >= 50) return "top-1/2 right-full mr-2 -translate-y-1/2";
  return "top-1/2 left-full ml-2 -translate-y-1/2";
}

function totalPotForAnimation(table: TableState) {
  if (table.last_result && (table.last_result.hand_id ?? table.hand_id) === table.hand_id) {
    return table.last_result.pot_cents;
  }
  return table.pot_cents;
}

function holeCardDealIndex(table: TableState, playerSeat: number, cardIndex: number) {
  const activeSeats = new Set(table.players.filter((player) => player.in_hand).map((player) => player.seat));
  const dealOrder: number[] = [];
  for (let offset = 1; offset <= tableSeatCount; offset++) {
    const seat = (table.dealer_seat + offset + tableSeatCount) % tableSeatCount;
    if (activeSeats.has(seat)) dealOrder.push(seat);
  }
  const playerIndex = dealOrder.indexOf(playerSeat);
  if (playerIndex < 0) return cardIndex;
  return playerIndex + cardIndex * dealOrder.length;
}

function seatPositions(table: TableState, seat: number) {
  const positions: Array<{ code: string; label: string }> = [];
  if (seat === table.dealer_seat) positions.push({ code: "BTN", label: "庄位" });
  if (seat === table.small_blind_seat) positions.push({ code: "SB", label: "小盲" });
  if (seat === table.big_blind_seat) positions.push({ code: "BB", label: "大盲" });
  return positions;
}

function playerActionLabel(player: Player) {
  if (player.ready) return "已准备";
  switch (player.last_action) {
    case "fold": return "弃牌";
    case "check": return "过牌";
    case "call": return "跟注";
    case "bet": return "下注";
    case "raise": return (player.last_action_bet_level || 0) >= 3 ? `${player.last_action_bet_level}-bet` : "加注";
    case "all_in": return "All-in";
    default: return "";
  }
}

function aggressiveActionLabel(table: TableState) {
  if (table.allowed_actions.can_bet) return "下注至";
  const nextLevel = (table.bet_level || 0) + 1;
  return nextLevel >= 3 ? `${nextLevel}-bet 至` : "加注至";
}

function betStatusLabel(table: TableState) {
  const level = table.bet_level || 0;
  if (level >= 3) return `${level}-bet`;
  if (level === 2) return "已加注";
  return table.street === "preflop" ? "大盲" : "已下注";
}

function streetLabel(street: string) {
  return ({ waiting: "等待开局", preflop: "翻牌前", flop: "翻牌", turn: "转牌", river: "河牌", complete: "本手结束" } as Record<string, string>)[street] || street;
}

function resultMessage(message: string) {
  return message
    .replace(/ win at showdown$/, " 摊牌获胜")
    .replace(/ wins$/, " 获胜")
    .replace(/^Showdown$/, "摊牌");
}

function quickAmount(table: TableState, ratio: number) {
  const allowed = table.allowed_actions;
  const target = table.current_bet_cents === 0
    ? Math.round(table.pot_cents * ratio / 50) * 50
    : table.current_bet_cents + Math.round(table.pot_cents * ratio / 50) * 50;
  return clamp(target, allowed.min_raise_to_cents, allowed.max_raise_to_cents);
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

function money(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2 }).format((cents || 0) / 100);
}

function hostOf(value: string) {
  try { return new URL(value).host; } catch { return value; }
}

function statusLabel(status: string) {
  return ({ completed: "已完成", pending: "处理中", manual_review: "需核对", compensated: "已退回" } as Record<string, string>)[status] || status;
}
