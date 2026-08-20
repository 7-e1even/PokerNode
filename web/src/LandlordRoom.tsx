import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEventHandler } from "react";
import { ArrowLeft, Check, CircleDollarSign, CircleHelp, Clock3, Crown, DoorOpen, LogOut, MoreHorizontal, Play, Spade } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Slider } from "@/components/ui/slider";
import { Spinner } from "@/components/ui/spinner";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { createSnapshotGate, type SnapshotGate } from "@/lib/snapshot-gate";
import { api, post } from "./api";
import { BindPanel } from "./PokerRoom";
import type { Balance, Card as GameCard, LandlordPlayer, LandlordTableEnvelope, LandlordTableState, Membership, Space, TableSummary, User } from "./types";

interface Props {
  user: User;
  initialSpace: Space;
  initialTable: TableSummary;
  onBack: () => void;
}

export default function LandlordRoom({ user, initialSpace, initialTable, onBack }: Props) {
  const [space, setSpace] = useState(initialSpace);
  const [membership, setMembership] = useState<Membership | null>(null);
  const [table, setTable] = useState<LandlordTableState | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [connection, setConnection] = useState<"connecting" | "live" | "polling" | "offline">("connecting");
  const [rulesOpen, setRulesOpen] = useState(false);
  const onBackRef = useRef(onBack);
  const snapshotGate = useRef(createSnapshotGate()).current;

  useEffect(() => { onBackRef.current = onBack; }, [onBack]);
  useEffect(() => { setTable(null); setConnection("connecting"); }, [initialTable.id]);

  const loadBalance = useCallback(async () => {
    if (!space.is_bound) return;
    try {
      const result = await api<{ balance: Balance }>(`/api/spaces/${space.id}/balance`);
      setBalance(result.balance);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "余额读取失败");
    }
  }, [space.id, space.is_bound]);

  const loadRoom = useCallback(async () => {
    try {
      const detail = await api<{ space: Space; membership: Membership }>(`/api/spaces/${space.id}`);
      setSpace(detail.space);
      setMembership(detail.membership);
      if (detail.space.is_bound) void loadBalance();
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "牌桌加载失败");
    }
  }, [loadBalance, space.id]);

  useEffect(() => { void loadRoom(); }, [loadRoom]);

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const socketURL = `${protocol}//${window.location.host}/api/spaces/${space.id}/tables/${initialTable.id}/ws`;
    const tableURL = `/api/spaces/${space.id}/tables/${initialTable.id}`;
    let disposed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let pollTimer: number | undefined;
    let attempts = 0;
    let pollInFlight = false;

    async function refresh() {
      if (disposed || pollInFlight) return;
      const snapshotToken = snapshotGate.beginRead();
      if (snapshotToken === null) return;
      pollInFlight = true;
      try {
        const result = await api<LandlordTableEnvelope>(tableURL);
        if (!disposed && snapshotGate.acceptRead(snapshotToken)) {
          setTable(result.table);
          if (socket?.readyState !== WebSocket.OPEN) setConnection("polling");
        }
      } catch {
        if (!disposed && socket?.readyState !== WebSocket.OPEN) setConnection("offline");
      } finally {
        pollInFlight = false;
      }
    }

    function startPolling() {
      if (pollTimer !== undefined) return;
      void refresh();
      pollTimer = window.setInterval(() => void refresh(), 2_000);
    }

    function connect() {
      if (disposed) return;
      const next = new WebSocket(socketURL);
      socket = next;
      next.onopen = () => {
        attempts = 0;
        if (pollTimer !== undefined) window.clearInterval(pollTimer);
        pollTimer = undefined;
        setConnection("live");
        void refresh();
      };
      next.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          if (message.type === "table") {
            snapshotGate.acceptPush();
            setTable(message.table);
          }
          if (message.type === "table_deleted") {
            toast.info("牌桌已被管理员删除");
            onBackRef.current();
          }
        } catch { /* ignore malformed realtime messages */ }
      };
      next.onclose = () => {
        if (disposed) return;
        socket = null;
        startPolling();
        const delay = Math.min(1_000 * 2 ** attempts, 15_000);
        attempts += 1;
        reconnectTimer = window.setTimeout(connect, delay);
      };
      next.onerror = () => next.close();
    }

    connect();
    return () => {
      disposed = true;
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      if (pollTimer !== undefined) window.clearInterval(pollTimer);
      socket?.close();
    };
  }, [initialTable.id, snapshotGate, space.id]);

  const actingPlayer = table?.players.find((player) => player.seat === table.acting_seat);

  return (
    <main className="table-room landlord-room flex h-svh min-h-0 flex-col overflow-hidden">
      <section className="table-room__canvas relative min-h-0 flex-1 overflow-hidden">
        <div className="table-room-hud landlord-room-hud pointer-events-none absolute inset-x-0 top-0 z-20 flex items-start justify-between gap-2">
          <div className="landlord-room-hud__primary pointer-events-none flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <div className="table-room-hud__identity landlord-room-hud__identity pointer-events-auto flex min-w-0 items-center p-1 pr-3">
              <Button className="rounded-full" size="icon" variant="ghost" onClick={onBack} aria-label="返回频道"><ArrowLeft /></Button>
              <BrandMark className="size-7 shrink-0" aria-hidden="true" />
              <div className="min-w-0 pl-1">
                <h1 className="truncate font-heading text-sm font-semibold">{table?.name || initialTable.name}</h1>
                <p className="hidden truncate text-xs text-muted-foreground md:block">斗地主 · 底分 {money(table?.base_stake_cents ?? initialTable.base_stake_cents ?? 0)}</p>
              </div>
            </div>

            {table && (
              <div className="table-room-hud__status landlord-room-hud__status pointer-events-auto flex items-center gap-2 whitespace-nowrap px-2 py-1" aria-live="polite">
                <Badge variant="secondary">第 {table.hand_id || 1} 局</Badge>
                <strong className="px-1 text-xs">{phaseLabel(table.phase)}</strong>
                <Separator orientation="vertical" className="h-4" />
                <span className="truncate text-xs text-muted-foreground">
                  {table.acting_seat >= 0 ? `${actingPlayer?.name || "玩家"}行动` : table.phase === "waiting" ? `${table.players.length}/3 人已入座` : "等待下一局"}
                </span>
                {table.highest_bid > 0 && <Badge variant="outline" className="hidden sm:inline-flex">叫分 {table.highest_bid}</Badge>}
                {table.multiplier > 1 && <Badge className="hidden sm:inline-flex">× {table.multiplier}</Badge>}
              </div>
            )}
          </div>

          <div className="table-room-hud__actions landlord-room-hud__actions pointer-events-auto flex shrink-0 items-center gap-1">
            <Badge variant={connection === "live" ? "secondary" : connection === "offline" ? "destructive" : "outline"} className="hidden lg:inline-flex" aria-live="polite"><span className={cn("size-1.5 rounded-full", connection === "live" ? "bg-foreground" : "bg-current")} aria-hidden="true" />{connection === "live" ? "实时" : connection === "polling" ? "自动同步" : connection === "connecting" ? "连接中" : "已断开"}</Badge>
            {space.is_bound && (
              <Button className="rounded-full" size="sm" variant="ghost" onClick={() => void loadBalance()} aria-label={balance ? `余额 ${money(balance.cents)}` : "刷新余额"}>
                <CircleDollarSign data-icon="inline-start" />
                <span className="hidden md:inline">{balance ? money(balance.cents) : "—"}</span>
              </Button>
            )}
            <Button className="rounded-full" size="sm" variant="ghost" onClick={onBack} aria-label="离开牌桌">
              <DoorOpen data-icon="inline-start" />
              <span className="hidden sm:inline">离开牌桌</span>
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button className="rounded-full" size="icon" variant="ghost" aria-label="更多牌桌操作"><MoreHorizontal /></Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-44">
                <DropdownMenuGroup>
                  <DropdownMenuItem onSelect={() => setRulesOpen(true)}><CircleHelp />斗地主规则</DropdownMenuItem>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>

        {table ? <LandlordTable table={table} user={user} spaceID={space.id} tableID={initialTable.id} onChanged={setTable} snapshotGate={snapshotGate} onBalanceChanged={() => void loadBalance()} /> : <LandlordLoading />}
        {!space.is_bound && membership && <BindPanel space={space} onBound={(nextMembership, nextBalance) => { setMembership(nextMembership); setBalance(nextBalance); setSpace((current) => ({ ...current, is_bound: true })); toast.success("个人凭证已绑定"); }} />}
      </section>

      <LandlordRules open={rulesOpen} onClose={() => setRulesOpen(false)} />
    </main>
  );
}

function LandlordTable({ table, user, spaceID, tableID, onChanged, snapshotGate, onBalanceChanged }: {
  table: LandlordTableState;
  user: User;
  spaceID: string;
  tableID: string;
  onChanged: (table: LandlordTableState) => void;
  snapshotGate: SnapshotGate;
  onBalanceChanged: () => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const [buyIn, setBuyIn] = useState(10_000);
  const [busy, setBusy] = useState(false);
  const dragSelection = useRef<{ selecting: boolean; visited: Set<string> } | null>(null);
  const pointerHandledCard = useRef<string | null>(null);
  const seconds = useCountdown(table.action_deadline_at, table.turn_id);
  const viewer = table.players.find((player) => player.user_id === user.id);
  const seated = table.viewer_seat >= 0;
  const opponents = table.players.filter((player) => player.user_id !== user.id);
  const selectedCards = (viewer?.cards || []).filter((card) => selected.includes(cardKey(card)));
  const viewerCards = [...(viewer?.cards || [])].sort((left, right) => right.rank - left.rank || right.suit - left.suit);
  const bottomCards = table.bottom || [];
  const lastPlay = table.last_play || [];
  const waitingForDeal = table.phase === "waiting";
  const lastPlayOrigin = table.last_play_seat === viewer?.seat
    ? "self"
    : table.last_play_seat === opponents[0]?.seat
      ? "left"
      : table.last_play_seat === opponents[1]?.seat
        ? "right"
        : "center";

  useEffect(() => { setSelected([]); }, [table.hand_id, table.turn_id]);

  useEffect(() => {
    const stopDragging = () => {
      dragSelection.current = null;
      window.setTimeout(() => { pointerHandledCard.current = null; }, 0);
    };
    window.addEventListener("pointerup", stopDragging);
    window.addEventListener("pointercancel", stopDragging);
    return () => {
      window.removeEventListener("pointerup", stopDragging);
      window.removeEventListener("pointercancel", stopDragging);
    };
  }, []);

  async function run(path: string, body?: unknown, balanceChanged = false) {
    const snapshotToken = snapshotGate.beginMutation();
    setBusy(true);
    try {
      const requestBody = path === "action" && body && typeof body === "object"
        ? { ...body, expected_turn_id: table.turn_id }
        : body;
      const result = await post<LandlordTableEnvelope>(`/api/spaces/${spaceID}/tables/${tableID}/${path}`, requestBody);
      if (result.table && snapshotGate.finishMutation(snapshotToken)) onChanged(result.table);
      else snapshotGate.cancelMutation(snapshotToken);
      setSelected([]);
      if (result.notice) toast.success(result.notice);
      if (balanceChanged) onBalanceChanged();
    } catch (caught) {
      snapshotGate.cancelMutation(snapshotToken);
      toast.error(caught instanceof Error ? caught.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  function toggleCard(card: GameCard) {
    const key = cardKey(card);
    setSelected((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key]);
  }

  function setCardSelected(key: string, selecting: boolean) {
    setSelected((current) => selecting
      ? current.includes(key) ? current : [...current, key]
      : current.filter((item) => item !== key));
  }

  function beginCardDrag(card: GameCard) {
    const key = cardKey(card);
    const selecting = !selected.includes(key);
    pointerHandledCard.current = key;
    dragSelection.current = { selecting, visited: new Set([key]) };
    setCardSelected(key, selecting);
  }

  function clickCard(card: GameCard) {
    if (pointerHandledCard.current === cardKey(card)) return;
    toggleCard(card);
  }

  function continueCardDrag(card: GameCard, buttons: number) {
    const drag = dragSelection.current;
    if (!drag || (buttons & 1) === 0) return;
    const key = cardKey(card);
    if (drag.visited.has(key)) return;
    drag.visited.add(key);
    setCardSelected(key, drag.selecting);
  }

  function continueTouchDrag(clientX: number, clientY: number) {
    const drag = dragSelection.current;
    if (!drag) return;
    const target = document.elementFromPoint(clientX, clientY)?.closest<HTMLElement>("[data-landlord-card]");
    const key = target?.dataset.landlordCard;
    if (!key || drag.visited.has(key)) return;
    drag.visited.add(key);
    setCardSelected(key, drag.selecting);
  }

  return (
    <div className="landlord-stage">
      {seated && table.phase !== "waiting" && <CardCounter counts={table.remaining_counts || {}} />}

      <div className="landlord-felt">
        <div className="landlord-table-mark" aria-hidden="true"><Crown /><span>POKERNODE</span></div>

        <div className="landlord-seat-position landlord-seat-position--left">
          {opponents[0] ? <PlayerPanel player={opponents[0]} side="left" waitingForDeal={waitingForDeal} showReady={table.phase === "waiting" || table.phase === "complete"} /> : <PlayerPlaceholder label="等待玩家" side="left" />}
        </div>
        <div className="landlord-seat-position landlord-seat-position--right">
          {opponents[1] ? <PlayerPanel player={opponents[1]} side="right" waitingForDeal={waitingForDeal} showReady={table.phase === "waiting" || table.phase === "complete"} /> : <PlayerPlaceholder label="等待玩家" side="right" />}
        </div>

        <div className="landlord-center-play">
          <div className="landlord-bottom-zone">
            <span>地主底牌</span>
            <CardRow cards={bottomCards} hiddenCount={table.phase === "waiting" || (table.landlord_seat < 0 && table.phase === "bidding") ? 3 : 0} compact motion="reveal" animationKey={table.hand_id} />
          </div>
          <div className="landlord-last-play">
            {table.phase !== "waiting" && <span>{table.trick_open ? `${table.players.find((player) => player.seat === table.last_play_seat)?.name || "上一家"} 出牌` : "桌面出牌区"}</span>}
            {lastPlay.length > 0 ? <CardRow cards={lastPlay} compact motion="play" motionOrigin={lastPlayOrigin} animationKey={table.turn_id} /> : <strong>{table.phase === "waiting" ? "等待玩家加入" : "等待首家出牌"}</strong>}
          </div>
          {table.last_result && <Badge variant="secondary" className="landlord-result-banner">{table.last_result.message} · 每家 {money(table.last_result.stake_cents)}</Badge>}
        </div>

        <div className="landlord-self-area">
          {viewer ? <>
            <PlayerPanel player={viewer} viewer waitingForDeal={waitingForDeal} showReady={table.phase === "waiting" || table.phase === "complete"} />
            <div className="landlord-hand" aria-label={`你的手牌，共 ${viewer.card_count} 张`}>
              <div className="landlord-hand-row" style={{ "--hand-count": Math.max(viewerCards.length, 1) } as CSSProperties} onPointerMove={(event) => { if (event.pointerType !== "mouse") continueTouchDrag(event.clientX, event.clientY); }}>
                {viewerCards.map((card, index, cards) => <LandlordCard key={`${table.hand_id}:${cardKey(card)}`} card={card} selected={selected.includes(cardKey(card))} disabled={!table.allowed_actions.can_play || busy} onClick={() => clickCard(card)} onPointerDown={(event) => { if (event.button === 0) { event.preventDefault(); beginCardDrag(card); } }} onPointerEnter={(event) => continueCardDrag(card, event.buttons)} motion="deal" motionIndex={index} motionX={`${((cards.length - 1) / 2 - index) * 2.1}rem`} />)}
                {viewer.card_count === 0 && !waitingForDeal && <span className="landlord-hand-empty py-8 text-sm text-muted-foreground">{table.phase === "complete" ? "本局手牌已出完" : "等待发牌"}</span>}
              </div>
            </div>
          </> : <div className="landlord-spectator-seat"><Spade /><span><strong>你的座位</strong><small>加入后，手牌会显示在这里</small></span></div>}
        </div>
      </div>

      <div className={cn("table-action-surface landlord-action-dock", !seated && "landlord-action-dock--join", (!seated || !table.allowed_actions.can_act) && "landlord-action-dock--passive")} role="toolbar" aria-label="牌局操作">
        {!seated && <><div className="landlord-buy-in flex min-w-44 flex-col gap-1 px-2"><div className="flex items-center justify-between text-xs"><span className="text-muted-foreground">坐下买入</span><strong>{money(buyIn)}</strong></div><Slider aria-label="买入金额" min={2_000} max={100_000} step={500} value={[buyIn]} onValueChange={(value) => setBuyIn(value[0])} /></div><Button className="landlord-primary-action rounded-full" size="lg" disabled={busy} onClick={() => void run("join", { buy_in_cents: buyIn }, true)}>{busy && <Spinner data-icon="inline-start" />}{busy ? "处理中…" : "加入牌桌"}</Button></>}
        {seated && table.allowed_actions.can_bid && <><Badge variant="outline" className="landlord-action-timer"><Clock3 data-icon="inline-start" />{seconds ?? table.action_timeout_seconds}</Badge><Button size="lg" variant="outline" className="rounded-full" disabled={busy} onClick={() => void run("action", { action: "bid", bid: 0 })}>不叫</Button>{[1, 2, 3].filter((bid) => bid >= table.allowed_actions.min_bid).map((bid) => <Button key={bid} size="lg" className="landlord-primary-action rounded-full" disabled={busy} onClick={() => void run("action", { action: "bid", bid })}>{bid} 分</Button>)}</>}
        {seated && table.allowed_actions.can_play && <><span className="landlord-selection-hint">按住牌面拖动多选</span><Badge variant="outline" className="landlord-action-timer"><Clock3 data-icon="inline-start" />{seconds ?? table.action_timeout_seconds}</Badge>{table.allowed_actions.can_pass && <Button size="lg" variant="outline" className="rounded-full" disabled={busy} onClick={() => void run("action", { action: "pass" })}>不出</Button>}<Button size="lg" className="landlord-primary-action min-w-28 rounded-full" disabled={busy || selectedCards.length === 0} onClick={() => void run("action", { action: "play", cards: selectedCards })}><Play data-icon="inline-start" />出牌 {selectedCards.length > 0 ? `(${selectedCards.length})` : ""}</Button></>}
        {seated && !table.allowed_actions.can_act && !table.can_start && table.can_leave && <><span className="px-2 text-sm text-muted-foreground">等待玩家 {table.players.length}/3</span><Button size="lg" variant="outline" className="rounded-full" disabled={busy} onClick={() => void run("leave", {}, true)}><LogOut data-icon="inline-start" />结算离桌</Button></>}
        {seated && table.can_start && <><div className="px-2 text-sm text-muted-foreground">{viewer?.ready ? table.players.length < 3 ? `等待玩家 ${table.players.length}/3` : `等待其他玩家 · 已准备 ${table.players.filter((player) => player.ready).length}/3` : table.players.length < 3 ? `等待玩家 ${table.players.length}/3` : `已准备 ${table.players.filter((player) => player.ready).length}/3`}</div>{table.can_leave && <Button className="rounded-full" variant="ghost" disabled={busy} onClick={() => void run("leave", {}, true)}><LogOut data-icon="inline-start" />结算离桌</Button>}{!viewer?.ready && <Button size="lg" className="landlord-primary-action rounded-full" disabled={busy} onClick={() => void run("ready")}>{busy ? <Spinner data-icon="inline-start" /> : <Check data-icon="inline-start" />}{busy ? "处理中…" : "准备"}</Button>}</>}
      </div>
    </div>
  );
}

function PlayerPanel({ player, side, viewer = false, waitingForDeal = false, showReady = false }: { player: LandlordPlayer; side?: "left" | "right"; viewer?: boolean; waitingForDeal?: boolean; showReady?: boolean }) {
  return (
    <div className={cn("landlord-player", side ? `landlord-player--${side}` : "landlord-self-player", player.is_acting && "landlord-player--acting")}>
      <div className="landlord-player-copy"><div><strong>{player.name}{viewer ? "（我）" : ""}</strong>{player.landlord && <Badge variant="secondary" className="landlord-player-landlord" aria-label="地主"><Crown data-icon="inline-start" /><span>地主</span></Badge>}</div><p>{money(player.stack_cents)}{player.bid > 0 ? ` · ${player.bid} 分` : ""}</p></div>
      <div className="landlord-card-count" aria-label={waitingForDeal ? "等待发牌" : `剩余 ${player.card_count} 张`}><i aria-hidden="true" /><strong>{waitingForDeal ? "—" : player.card_count}</strong><span>{waitingForDeal ? "待发" : "张"}</span></div>
      {showReady && <Badge variant={player.ready ? "secondary" : "outline"} className="landlord-player-ready data-[ready=true]:border-success data-[ready=true]:bg-success data-[ready=true]:text-success-foreground" data-ready={player.ready ? "true" : undefined}>{player.ready ? "已准备" : "未准备"}</Badge>}
      {player.is_acting && <span className="landlord-player-turn">行动中</span>}
    </div>
  );
}

function CardCounter({ counts }: { counts: Record<string, number> }) {
  return (
    <div className="landlord-card-counter" role="region" aria-label="记牌器">
      <Badge variant="secondary" className="landlord-card-counter__label">记牌器</Badge>
      {CARD_COUNTER_RANKS.map(({ rank, label }) => {
        const count = counts[String(rank)] || 0;
        return <div key={rank} className="landlord-card-counter__item" data-empty={count === 0 ? "true" : undefined} aria-label={`${label}剩余${count}张`}><span>{label}</span><strong>{count}</strong></div>;
      })}
    </div>
  );
}

function PlayerPlaceholder({ label, side }: { label: string; side: "left" | "right" }) {
  return <div className={cn("landlord-player landlord-player--empty", `landlord-player--${side}`)}><div className="landlord-player-copy"><strong>{label}</strong><p>空位</p></div><div className="landlord-card-count"><strong>—</strong><span>待发</span></div></div>;
}

function CardRow({ cards, hiddenCount = 0, compact = false, motion, motionOrigin, animationKey = 0 }: { cards: GameCard[]; hiddenCount?: number; compact?: boolean; motion?: "play" | "reveal"; motionOrigin?: "left" | "right" | "self" | "center"; animationKey?: string | number }) {
  if (hiddenCount > 0) return <div className="flex">{Array.from({ length: hiddenCount }, (_, index) => <CardBack key={index} compact={compact} style={{ marginLeft: index === 0 ? 0 : "-0.75rem" }} />)}</div>;
  return <div className="flex">{cards.map((card, index) => <LandlordCard key={`${animationKey}:${cardKey(card)}:${index}`} card={card} compact={compact} disabled motion={motion} motionOrigin={motionOrigin} motionIndex={index} style={{ marginLeft: index === 0 ? 0 : compact ? "-0.65rem" : "-1.25rem" }} />)}</div>;
}

function LandlordCard({ card, selected = false, compact = false, disabled = false, onClick, onPointerDown, onPointerEnter, style, motion, motionOrigin, motionIndex = 0, motionX = "0rem" }: { card: GameCard; selected?: boolean; compact?: boolean; disabled?: boolean; onClick?: () => void; onPointerDown?: PointerEventHandler<HTMLButtonElement>; onPointerEnter?: PointerEventHandler<HTMLButtonElement>; style?: CSSProperties; motion?: "deal" | "play" | "reveal"; motionOrigin?: "left" | "right" | "self" | "center"; motionIndex?: number; motionX?: string }) {
  const joker = card.rank >= 16;
  const red = card.suit === 1 || card.suit === 2 || card.rank === 17;
  const rank = rankLabel(card.rank);
  const suit = joker ? "王" : ["♣", "♦", "♥", "♠"][card.suit];
  const motionStyle = { ...style, "--motion-index": motionIndex, "--motion-x": motionX, zIndex: motionIndex + 1 } as CSSProperties;
  return (
    <button type="button" className={cn("landlord-playing-card", compact ? "landlord-playing-card--compact" : "landlord-playing-card--hand", red && "text-destructive", selected && "landlord-playing-card--selected", disabled && "cursor-default")} disabled={disabled} onClick={onClick} onPointerDown={onPointerDown} onPointerEnter={onPointerEnter} style={motionStyle} data-motion={motion} data-origin={motionOrigin} data-landlord-card={cardKey(card)} aria-pressed={onClick ? selected : undefined} aria-label={`${rank}${suit}`}>
      <span className="landlord-card-turn">
        <span className="landlord-card-face"><strong className="leading-none">{rank}</strong><span className="leading-none">{suit}</span>{joker && <span className="mt-auto self-end text-[0.55rem] font-medium uppercase">Joker</span>}</span>
        {motion === "reveal" && <span className="landlord-card-motion-back" aria-hidden="true"><BrandMark /></span>}
      </span>
    </button>
  );
}

function CardBack({ compact, style }: { compact?: boolean; style?: CSSProperties }) {
  return <div className={cn("landlord-card-back", compact ? "landlord-playing-card--compact" : "landlord-playing-card--hand")} style={style}><BrandMark aria-hidden="true" /></div>;
}

function LandlordRules({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader><DialogTitle>斗地主规则</DialogTitle><DialogDescription>当前牌桌采用三人叫分玩法，筹码以美元美分结算。</DialogDescription></DialogHeader>
        <div className="flex flex-col gap-4 text-sm leading-6">
          <div><strong>叫地主</strong><p className="text-muted-foreground">每人依次选择不叫或 1–3 分，最高分玩家成为地主并获得 3 张底牌；无人叫分会重新发牌。</p></div>
          <div><strong>出牌</strong><p className="text-muted-foreground">支持单牌、对子、三张、三带一/二、顺子、连对、飞机、四带二、炸弹和王炸。同轮需用相同牌型与张数压过上一手，炸弹和王炸除外。</p></div>
          <div><strong>结算</strong><p className="text-muted-foreground">每家输赢为底分 × 叫分 × 倍数。炸弹、王炸和春天各翻倍；实际赔付不超过玩家桌上筹码。</p></div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function LandlordLoading() {
  return <div className="mx-auto grid h-full max-w-6xl grid-rows-[auto_1fr_auto] gap-4 p-6"><div className="grid grid-cols-2 gap-3"><Skeleton className="h-20" /><Skeleton className="h-20" /></div><Skeleton className="min-h-48" /><Skeleton className="h-36" /></div>;
}

function useCountdown(deadline: number, turnID: number) {
  const [seconds, setSeconds] = useState<number | null>(null);
  useEffect(() => {
    if (!deadline) { setSeconds(null); return; }
    const update = () => setSeconds(Math.max(0, Math.ceil((deadline - Date.now()) / 1_000)));
    update();
    const timer = window.setInterval(update, 250);
    return () => window.clearInterval(timer);
  }, [deadline, turnID]);
  return seconds;
}

function cardKey(card: GameCard) { return `${card.rank}:${card.suit}`; }

const CARD_COUNTER_RANKS = [
  { rank: 17, label: "大" }, { rank: 16, label: "小" }, { rank: 15, label: "2" },
  { rank: 14, label: "A" }, { rank: 13, label: "K" }, { rank: 12, label: "Q" },
  { rank: 11, label: "J" }, { rank: 10, label: "10" }, { rank: 9, label: "9" },
  { rank: 8, label: "8" }, { rank: 7, label: "7" }, { rank: 6, label: "6" },
  { rank: 5, label: "5" }, { rank: 4, label: "4" }, { rank: 3, label: "3" },
] as const;

function rankLabel(rank: number) {
  return ({ 10: "10", 11: "J", 12: "Q", 13: "K", 14: "A", 15: "2", 16: "小", 17: "大" } as Record<number, string>)[rank] || String(rank);
}

function phaseLabel(phase: LandlordTableState["phase"]) {
  return ({ waiting: "等待入座", bidding: "叫地主", playing: "出牌中", complete: "本局结束" } as const)[phase];
}

function money(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2 }).format(cents / 100);
}
