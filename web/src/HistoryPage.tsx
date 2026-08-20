import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, CircleDollarSign, Crown, History, ReceiptText } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BrandMark } from "@/components/brand-mark";
import { cn } from "@/lib/utils";
import { api } from "./api";
import type { Card as PokerCard, HandHistory, HandResultPlayer, Space, User, WalletOperation } from "./types";

interface Props {
  user: User;
  space: Space;
  onBack: () => void;
}

export default function HistoryPage({ user, space, onBack }: Props) {
  const [hands, setHands] = useState<HandHistory[]>([]);
  const [operations, setOperations] = useState<WalletOperation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    Promise.all([
      api<{ hands: HandHistory[] }>(`/api/spaces/${space.id}/hands`),
      api<{ operations: WalletOperation[] }>(`/api/spaces/${space.id}/operations`),
    ]).then(([handResult, operationResult]) => {
      if (cancelled) return;
      setHands(handResult.hands || []);
      setOperations(operationResult.operations || []);
    }).catch((caught) => {
      if (!cancelled) setError(caught instanceof Error ? caught.message : "读取记录失败");
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [space.id]);

  const netFunds = useMemo(() => operations.reduce((total, operation) => total + signedOperationCents(operation), 0), [operations]);

  return (
    <main className="min-h-svh bg-muted/30">
      <header className="border-b bg-background">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-4 sm:px-6">
          <Button size="icon" variant="ghost" onClick={onBack} aria-label="返回牌桌"><ArrowLeft /></Button>
          <BrandMark className="size-9 shrink-0" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <h1 className="truncate font-heading text-lg font-semibold">牌局与资金记录</h1>
            <p className="truncate text-sm text-muted-foreground">{space.name} · 最近 50 条个人记录</p>
          </div>
          <Badge variant="outline">{user.display_name}</Badge>
        </div>
      </header>

      <div className="mx-auto flex max-w-6xl flex-col gap-5 px-4 py-6 sm:px-6">
        {error && <Alert variant="destructive"><AlertTitle>记录读取失败</AlertTitle><AlertDescription>{error}</AlertDescription></Alert>}
        <Tabs defaultValue="hands">
          <TabsList>
            <TabsTrigger value="hands"><History />牌局记录 <Badge variant="secondary">{hands.length}</Badge></TabsTrigger>
            <TabsTrigger value="funds"><CircleDollarSign />资金流水 <Badge variant="secondary">{operations.length}</Badge></TabsTrigger>
          </TabsList>

          <TabsContent value="hands" className="flex flex-col gap-4 pt-2">
            {loading ? <HandHistorySkeleton /> : hands.length === 0 ? <HistoryEmpty kind="hands" /> : hands.map((hand) => <HandCard key={`${hand.table_id}-${hand.hand_id}`} hand={hand} viewerID={user.id} />)}
          </TabsContent>

          <TabsContent value="funds" className="pt-2">
            {loading ? <Skeleton className="h-80 w-full" /> : operations.length === 0 ? <HistoryEmpty kind="funds" /> : (
              <Card>
                <CardHeader>
                  <CardTitle>资金流水</CardTitle>
                  <CardDescription>买入、离桌结算和管理员调账分开标记；牌桌输赢请以“牌局记录”为准。</CardDescription>
                  <CardAction><Badge variant="outline">记录净额 {signedMoney(netFunds)}</Badge></CardAction>
                </CardHeader>
                <CardContent>
                  <div className="flex flex-col gap-3 md:hidden">{operations.map((operation) => <OperationCard key={operation.id} operation={operation} />)}</div>
                  <div className="hidden md:block">
                    <Table>
                      <TableHeader><TableRow><TableHead>类型</TableHead><TableHead>牌桌</TableHead><TableHead>时间</TableHead><TableHead>备注</TableHead><TableHead>金额</TableHead><TableHead>状态</TableHead></TableRow></TableHeader>
                      <TableBody>
                        {operations.map((operation) => {
                          const cents = signedOperationCents(operation);
                          return (
                            <TableRow key={operation.id}>
                              <TableCell className="font-medium">{operationLabel(operation.kind)}</TableCell>
                              <TableCell className="font-mono text-xs text-muted-foreground">{tableLabel(operation.table_id)}</TableCell>
                              <TableCell className="text-muted-foreground">{formatTime(operation.created_at)}</TableCell>
                              <TableCell className="max-w-56 truncate text-muted-foreground">{operation.note || "—"}</TableCell>
                              <TableCell className={cn("font-mono font-medium", amountClass(cents))}>{signedMoney(cents)}</TableCell>
                              <TableCell><OperationStatus operation={operation} /></TableCell>
                            </TableRow>
                          );
                        })}
                      </TableBody>
                    </Table>
                  </div>
                </CardContent>
              </Card>
            )}
          </TabsContent>
        </Tabs>
      </div>
    </main>
  );
}

function HandCard({ hand, viewerID }: { hand: HandHistory; viewerID: number }) {
  const result = hand.table.last_result;
  const players = result?.players || [];
  const winners = players.filter((player) => player.payout_cents > 0).map((player) => player.name);
  const viewer = players.find((player) => player.user_id === viewerID);
  const board = result?.board || hand.table.board || [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>第 {hand.hand_id} 手 · {hand.table.name || tableLabel(hand.table_id)}</CardTitle>
        <CardDescription>{formatTime(hand.completed_at)} · {result?.showdown ? "摊牌结算" : "未摊牌结束"}</CardDescription>
        <CardAction><Badge variant="outline">底池 {money(result?.pot_cents || 0)}</Badge></CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-sm text-muted-foreground">公共牌</span>
          <CompactCards cards={board} emptyLabel="无公共牌" />
          <Separator orientation="vertical" className="hidden h-6 sm:block" />
          <Badge variant="secondary"><Crown />{winners.length > 0 ? winners.join("、") : "本手赢家"}</Badge>
        </div>
        <div className="flex flex-col gap-3 sm:hidden">{players.map((player) => <HandPlayerCard key={player.user_id} player={player} viewerID={viewerID} />)}</div>
        <div className="hidden sm:block">
          <Table>
            <TableHeader><TableRow><TableHead>玩家</TableHead><TableHead>手牌</TableHead><TableHead>投入底池</TableHead><TableHead>赢取</TableHead><TableHead>未跟注退回</TableHead><TableHead>本手盈亏</TableHead></TableRow></TableHeader>
            <TableBody>{players.map((player) => <HandPlayerRow key={player.user_id} player={player} viewerID={viewerID} />)}</TableBody>
          </Table>
        </div>
      </CardContent>
      <CardFooter className="justify-between gap-3 text-sm">
        <span className="text-muted-foreground">{result?.message ? resultMessage(result.message) : "本手已结算"}</span>
        {viewer && <strong className={cn("font-mono", amountClass(viewer.net_cents))}>你本手 {signedMoney(viewer.net_cents)}</strong>}
      </CardFooter>
    </Card>
  );
}

function HandPlayerRow({ player, viewerID }: { player: HandResultPlayer; viewerID: number }) {
  return (
    <TableRow>
      <TableCell className="font-medium">{player.name}{player.user_id === viewerID ? "（你）" : ""}{player.folded && <Badge className="ml-2" variant="outline">弃牌</Badge>}</TableCell>
      <TableCell><CompactCards cards={player.cards || []} emptyLabel="未亮牌" /></TableCell>
      <TableCell className="font-mono">{money(player.committed_cents)}</TableCell>
      <TableCell className="font-mono">{player.payout_cents > 0 ? money(player.payout_cents) : "—"}</TableCell>
      <TableCell className="font-mono">{player.refund_cents ? money(player.refund_cents) : "—"}</TableCell>
      <TableCell className={cn("font-mono font-medium", amountClass(player.net_cents))}>{signedMoney(player.net_cents)}</TableCell>
    </TableRow>
  );
}

function HandPlayerCard({ player, viewerID }: { player: HandResultPlayer; viewerID: number }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{player.name}{player.user_id === viewerID ? "（你）" : ""}</CardTitle>
        <CardDescription className="flex items-center gap-2"><CompactCards cards={player.cards || []} emptyLabel="未亮牌" />{player.folded && <Badge variant="outline">弃牌</Badge>}</CardDescription>
        <CardAction><strong className={cn("font-mono", amountClass(player.net_cents))}>{signedMoney(player.net_cents)}</strong></CardAction>
      </CardHeader>
      <CardContent className="grid grid-cols-3 gap-3 text-xs">
        <MoneyDetail label="投入底池" cents={player.committed_cents} />
        <MoneyDetail label="赢取" cents={player.payout_cents} empty />
        <MoneyDetail label="未跟注退回" cents={player.refund_cents || 0} empty />
      </CardContent>
    </Card>
  );
}

function OperationCard({ operation }: { operation: WalletOperation }) {
  const cents = signedOperationCents(operation);
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{operationLabel(operation.kind)}</CardTitle>
        <CardDescription>{tableLabel(operation.table_id)} · {formatTime(operation.created_at)}</CardDescription>
        <CardAction><strong className={cn("font-mono", amountClass(cents))}>{signedMoney(cents)}</strong></CardAction>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-3">
        <span className="min-w-0 truncate text-muted-foreground">{operation.note || "无备注"}</span>
        <OperationStatus operation={operation} />
      </CardContent>
    </Card>
  );
}

function OperationStatus({ operation }: { operation: WalletOperation }) {
  return <Badge variant={operation.status === "completed" ? "secondary" : operation.status === "manual_review" ? "destructive" : "outline"}>{statusLabel(operation.status)}</Badge>;
}

function MoneyDetail({ label, cents, empty = false }: { label: string; cents: number; empty?: boolean }) {
  return <span className="flex min-w-0 flex-col gap-1"><small className="truncate text-muted-foreground">{label}</small><strong className="truncate font-mono">{empty && cents === 0 ? "—" : money(cents)}</strong></span>;
}

function CompactCards({ cards, emptyLabel }: { cards: PokerCard[]; emptyLabel: string }) {
  if (cards.length === 0) return <span className="text-xs text-muted-foreground">{emptyLabel}</span>;
  return <span className="inline-flex gap-1">{cards.map((card, index) => <Badge key={`${card.rank}-${card.suit}-${index}`} variant="outline" className={cn("h-8 min-w-8 px-1.5 font-mono text-sm", card.suit === 1 || card.suit === 2 ? "text-destructive" : "text-foreground")}>{rankLabel(card.rank)}{suitLabel(card.suit)}</Badge>)}</span>;
}

function HistoryEmpty({ kind }: { kind: "hands" | "funds" }) {
  return (
    <Empty className="min-h-72">
      <EmptyHeader>
        <EmptyMedia variant="icon">{kind === "hands" ? <History /> : <ReceiptText />}</EmptyMedia>
        <EmptyTitle>{kind === "hands" ? "还没有牌局记录" : "还没有资金流水"}</EmptyTitle>
        <EmptyDescription>{kind === "hands" ? "下一手结束后，会保存公共牌、可见手牌、投入、派彩和未跟注退回。" : "完成买入、离桌结算或余额调整后，记录会显示在这里。"}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function HandHistorySkeleton() {
  return <div className="flex flex-col gap-4">{Array.from({ length: 3 }, (_, index) => <Skeleton key={index} className="h-64 w-full" />)}</div>;
}

function signedOperationCents(operation: WalletOperation) {
  return operation.kind === "buy_in" || operation.kind === "manual_debit" ? -operation.cents : operation.cents;
}

function operationLabel(kind: WalletOperation["kind"]) {
  return ({ buy_in: "买入牌桌", cash_out: "离桌结算", manual_credit: "管理员加款", manual_debit: "管理员扣款" } as const)[kind];
}

function tableLabel(tableID: string) {
  if (!tableID) return "频道余额";
  return tableID === "main" ? "默认桌" : tableID.slice(0, 8);
}

function statusLabel(status: string) {
  return ({ completed: "已完成", pending: "处理中", manual_review: "需核对", compensated: "已退回" } as Record<string, string>)[status] || status;
}

function resultMessage(message: string) {
  if (message.endsWith(" win at showdown")) return `${message.slice(0, -" win at showdown".length)} 摊牌获胜`;
  if (message.endsWith(" wins")) return `${message.slice(0, -" wins".length)} 获胜`;
  return message;
}

function rankLabel(rank: number) {
  return ({ 10: "10", 11: "J", 12: "Q", 13: "K", 14: "A" } as Record<number, string>)[rank] || String(rank);
}

function suitLabel(suit: number) {
  return ["♣", "♦", "♥", "♠"][suit] || "?";
}

function formatTime(value: string) {
  return new Date(value).toLocaleString();
}

function money(cents: number) {
  return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD" }).format(cents / 100);
}

function signedMoney(cents: number) {
  return `${cents > 0 ? "+" : ""}${money(cents)}`;
}

function amountClass(cents: number) {
  return cents > 0 ? "text-success" : cents < 0 ? "text-destructive" : "text-muted-foreground";
}
