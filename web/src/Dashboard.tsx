import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowRight, Bot, CheckCircle2, Copy, Crown, DoorOpen, Gamepad2, KeyRound, Link2, LogOut,
  Menu, Plus, RadioTower, ShieldCheck, Trophy,
} from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { BrandMark } from "@/components/brand-mark";
import { AccountCredentialsDialog } from "@/components/account-credentials-dialog";
import { MCPKeyDialog } from "@/components/mcp-key-dialog";
import { WeChatIcon } from "@/components/wechat-icon";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { api, post } from "./api";
import type { ChannelLeaderboardEntry, Space, User } from "./types";

interface Props {
  user: User;
  view: "ranking" | "channels";
  wechatLoginEnabled: boolean;
  onViewChange: (view: "ranking" | "channels") => void;
  onOpenSpace: (space: Space) => void;
  onOpenBindings: () => void;
  onOpenAdmin?: () => void;
  onUserUpdated: (user: User) => void;
  onLogout: () => void;
}

export default function Dashboard({ user, view, wechatLoginEnabled, onViewChange, onOpenSpace, onOpenBindings, onOpenAdmin, onUserUpdated, onLogout }: Props) {
  const [spaces, setSpaces] = useState<Space[]>([]);
  const [leaderboard, setLeaderboard] = useState<ChannelLeaderboardEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [leaderboardLoading, setLeaderboardLoading] = useState(true);
  const [dialog, setDialog] = useState<"create" | "join" | null>(null);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [mcpKeyOpen, setMCPKeyOpen] = useState(false);
  const [error, setError] = useState("");
  const [leaderboardError, setLeaderboardError] = useState("");

  useEffect(() => {
    api<{ spaces: Space[] }>("/api/spaces")
      .then((result) => setSpaces(result.spaces || []))
      .catch((caught) => setError(caught instanceof Error ? caught.message : "读取频道失败"))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    api<{ leaderboard: ChannelLeaderboardEntry[] }>("/api/leaderboard")
      .then((result) => setLeaderboard(result.leaderboard || []))
      .catch((caught) => setLeaderboardError(caught instanceof Error ? caught.message : "读取大厅排名失败"))
      .finally(() => setLeaderboardLoading(false));
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const result = params.get("wechat_link");
    if (!result) return;
    const messages: Record<string, string> = {
      success: "微信绑定成功，以后可以直接使用微信登录。",
      already_bound: "这个微信已绑定其他 PokerNode 账号。",
      account_bound: "当前账号已经绑定了另一个微信。",
      session_expired: "登录状态已过期，请重新登录后再绑定微信。",
      cancelled: "已取消微信授权。",
      invalid_state: "微信授权已失效，请重新发起绑定。",
      provider_failed: "微信授权失败，请稍后重试。",
      unavailable: "微信绑定尚未配置。",
      failed: "绑定微信失败，请稍后重试。",
    };
    if (result === "success") toast.success(messages[result]);
    else toast.error(messages[result] ?? messages.failed);
    params.delete("wechat_link");
    const query = params.toString();
    window.history.replaceState({}, "", `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`);
  }, []);

  async function logout() {
    await post("/api/auth/logout");
    onLogout();
  }

  function openSpace(space: Space) {
    setMobileOpen(false);
    onOpenSpace(space);
  }

  return (
    <div className="game-canvas flex min-h-svh flex-col">
      <GameHeader
        user={user}
        view={view}
        onViewChange={onViewChange}
        onOpenMenu={() => setMobileOpen(true)}
        onOpenBindings={onOpenBindings}
        onOpenAccount={() => setAccountOpen(true)}
        onOpenMCP={() => setMCPKeyOpen(true)}
        onOpenAdmin={onOpenAdmin}
        wechatLoginEnabled={wechatLoginEnabled}
        onLogout={() => void logout()}
      />

      <AccountCredentialsDialog user={user} open={accountOpen} onOpenChange={setAccountOpen} onUpdated={onUserUpdated} />
      <MCPKeyDialog open={mcpKeyOpen} onOpenChange={setMCPKeyOpen} />

      <div className="flex min-h-0 flex-1">
        <main id="main-content" className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          {view === "ranking" ? (
            <RankingView user={user} leaderboard={leaderboard} loading={leaderboardLoading} error={leaderboardError} />
          ) : (
            <ChannelPickerView spaces={spaces} loading={loading} error={error} onCreate={() => setDialog("create")} onJoin={() => setDialog("join")} onOpenSpace={openSpace} />
          )}
        </main>
      </div>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-72 p-0" showCloseButton={false}>
          <SheetHeader className="border-b"><SheetTitle>游戏大厅</SheetTitle><SheetDescription>选择功能或频道</SheetDescription></SheetHeader>
          <LobbySidebar
            className="flex min-h-0 flex-1 border-0"
            view={view}
            spaces={spaces}
            connected={!error && !leaderboardError}
            onViewChange={(nextView) => { setMobileOpen(false); onViewChange(nextView); }}
            onOpenSpace={openSpace}
            onCreate={() => { setMobileOpen(false); setDialog("create"); }}
            onJoin={() => { setMobileOpen(false); setDialog("join"); }}
          />
        </SheetContent>
      </Sheet>

      {dialog && (
        <SpaceDialog
          mode={dialog}
          onClose={() => setDialog(null)}
          onCreated={(space) => {
            setDialog(null);
            setSpaces((current) => [space, ...current.filter((item) => item.id !== space.id)]);
            onOpenSpace(space);
          }}
        />
      )}
    </div>
  );
}

function GameHeader({ user, view, onViewChange, onOpenMenu, onOpenBindings, onOpenAccount, onOpenMCP, onOpenAdmin, wechatLoginEnabled, onLogout }: {
  user: User;
  view: "ranking" | "channels";
  onViewChange: (view: "ranking" | "channels") => void;
  onOpenMenu: () => void;
  onOpenBindings: () => void;
  onOpenAccount: () => void;
  onOpenMCP: () => void;
  onOpenAdmin?: () => void;
  wechatLoginEnabled: boolean;
  onLogout: () => void;
}) {
  return (
    <header className="game-topbar flex h-16 shrink-0 items-center justify-between gap-4 px-3 sm:px-6">
      <div className="flex min-w-0 items-center gap-2 sm:gap-4">
        <Button size="icon" variant="secondary" className="min-h-11 min-w-11 md:hidden" onClick={onOpenMenu} aria-label="打开游戏导航"><Menu /></Button>
        <div className="flex min-w-0 items-center gap-3">
          <BrandMark className="size-10" aria-hidden="true" />
          <span className="hidden min-w-0 sm:block"><strong className="block truncate font-heading">PokerNode</strong><small className="block truncate">Friends Game Hall</small></span>
        </div>
        <nav className="hidden items-center gap-1 md:flex" aria-label="主导航">
          <Button variant={view === "ranking" ? "secondary" : "ghost"} onClick={() => onViewChange("ranking")}><Trophy data-icon="inline-start" />排行榜</Button>
          <Button variant={view === "channels" ? "secondary" : "ghost"} onClick={() => onViewChange("channels")}><Gamepad2 data-icon="inline-start" />进入平台</Button>
        </nav>
      </div>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="secondary" className="h-auto gap-2 p-1.5 pr-2"><Avatar className="size-8"><AvatarImage src={user.avatar_url} alt={user.display_name} /><AvatarFallback>{initials(user.display_name)}</AvatarFallback></Avatar><span className="hidden min-w-0 text-left sm:block"><strong className="block max-w-32 truncate text-xs">{user.display_name}</strong><small className="block text-xs text-muted-foreground">{roleLabel(user.role)}</small></span></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56"><DropdownMenuLabel><span className="block text-foreground">{user.display_name}</span><span className="block font-normal">@{user.username}</span></DropdownMenuLabel><DropdownMenuSeparator /><DropdownMenuGroup><DropdownMenuItem onSelect={onOpenAccount}><KeyRound />账号安全</DropdownMenuItem><DropdownMenuItem onSelect={onOpenMCP}><Bot />Agent MCP</DropdownMenuItem><DropdownMenuItem onSelect={onOpenBindings}><Link2 />频道账号</DropdownMenuItem>{onOpenAdmin && <DropdownMenuItem onSelect={onOpenAdmin}><ShieldCheck />打开运营后台</DropdownMenuItem>}{wechatLoginEnabled && (user.wechat_bound ? <DropdownMenuItem disabled><CheckCircle2 />微信已绑定</DropdownMenuItem> : <DropdownMenuItem onSelect={() => window.location.assign("/api/auth/wechat/link")}><WeChatIcon />绑定微信</DropdownMenuItem>)}<DropdownMenuItem variant="destructive" onSelect={onLogout}><LogOut />退出登录</DropdownMenuItem></DropdownMenuGroup></DropdownMenuContent>
      </DropdownMenu>
    </header>
  );
}

function LobbySidebar({ className, view, spaces, connected, onViewChange, onOpenSpace, onCreate, onJoin }: {
  className?: string;
  view: "ranking" | "channels";
  spaces: Space[];
  connected: boolean;
  onViewChange: (view: "ranking" | "channels") => void;
  onOpenSpace: (space: Space) => void;
  onCreate: () => void;
  onJoin: () => void;
}) {
  return (
    <aside className={cn("min-h-0 flex-col border-r bg-card", className)}>
      <div className="flex flex-col gap-2 p-4">
        <p className="text-xs font-medium text-muted-foreground">快速开始</p>
        <div className="grid grid-cols-2 gap-2"><Button className="min-h-11" onClick={onCreate}><Plus data-icon="inline-start" />创建</Button><Button className="min-h-11" variant="outline" onClick={onJoin}><DoorOpen data-icon="inline-start" />加入</Button></div>
      </div>
      <Separator />
      <div className="flex flex-col gap-1 p-3">
        <Button className="min-h-11 justify-start" variant={view === "ranking" ? "secondary" : "ghost"} onClick={() => onViewChange("ranking")}><Trophy data-icon="inline-start" />排行榜</Button>
        <Button className="min-h-11 justify-start" variant={view === "channels" ? "secondary" : "ghost"} onClick={() => onViewChange("channels")}><Gamepad2 data-icon="inline-start" />进入平台</Button>
      </div>
      <Separator />
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3"><LobbySidebarGroup label="我的频道" spaces={spaces} empty="还没有频道" onOpenSpace={onOpenSpace} /></div>
      </ScrollArea>
      <Separator />
      <div className="p-4"><div className="flex items-center gap-3 text-xs text-muted-foreground"><span className="relative flex size-2"><span className={cn("relative inline-flex size-2 rounded-full", connected ? "bg-primary" : "bg-destructive")} /></span><span>{connected ? "大厅数据已连接" : "大厅数据暂不可用"}</span></div></div>
    </aside>
  );
}

function LobbySidebarGroup({ label, spaces, empty, onOpenSpace }: {
  label: string;
  spaces: Space[];
  empty: string;
  onOpenSpace: (space: Space) => void;
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between px-1 pb-1"><span className="text-xs font-medium text-muted-foreground">{label}</span><Badge variant="outline">{spaces.length}</Badge></div>
      {spaces.length === 0 ? <p className="px-2 py-4 text-center text-xs text-muted-foreground">{empty}</p> : spaces.map((space) => (
        <Button key={space.id} className="h-auto justify-start py-2" variant="ghost" onClick={() => onOpenSpace(space)}><span className="game-room-avatar grid size-8 shrink-0 place-items-center rounded-lg font-heading font-semibold">{space.name.slice(0, 1).toUpperCase()}</span><span className="min-w-0 flex-1 text-left"><strong className="block truncate text-sm">{space.name}</strong><small className="block truncate text-xs text-muted-foreground">{space.can_manage ? "我管理的频道" : "已加入的频道"}</small></span></Button>
      ))}
    </div>
  );
}

function RankingView({ user, leaderboard, loading, error }: {
  user: User;
  leaderboard: ChannelLeaderboardEntry[];
  loading: boolean;
  error: string;
}) {
  return (
    <div className="player-lobby flex flex-1 overflow-auto">
      <div className="mx-auto flex w-full max-w-5xl flex-col px-4 py-7 sm:px-6 sm:py-10">
        <section className="flex flex-col gap-5" aria-labelledby="ranking-title">
          <header className="flex flex-col gap-2">
            <h1 id="ranking-title" className="font-heading text-3xl font-semibold tracking-tight">排行榜</h1>
            <p className="text-sm text-muted-foreground">查看你已加入频道的已结算战绩。</p>
          </header>

          <LobbyLeaderboard entries={leaderboard} currentUserID={user.id} loading={loading} error={error} />
        </section>
      </div>
    </div>
  );
}

function ChannelPickerView({ spaces, loading, error, onCreate, onJoin, onOpenSpace }: {
  spaces: Space[];
  loading: boolean;
  error: string;
  onCreate: () => void;
  onJoin: () => void;
  onOpenSpace: (space: Space) => void;
}) {
  return (
    <div className="player-lobby flex flex-1 overflow-auto">
      <div className="mx-auto flex w-full max-w-7xl flex-col px-4 py-7 sm:px-6 sm:py-10 lg:px-8">
        <section className="flex flex-col gap-5" aria-labelledby="platform-title">
          <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
            <div className="flex max-w-2xl flex-col gap-3">
              <Badge variant="outline" className="w-fit"><Gamepad2 data-icon="inline-start" />PokerNode Platform</Badge>
              <div className="flex items-center gap-2">
                <h1 id="platform-title" className="font-heading text-3xl font-semibold tracking-tight sm:text-4xl">选择频道</h1>
                <Badge variant="secondary">{spaces.length}</Badge>
              </div>
              <p className="text-sm leading-6 text-muted-foreground">进入频道后选择牌桌，查看在线牌友和频道内的实时排名。</p>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button className="min-h-11 lg:min-h-8" variant="outline" onClick={onJoin}><DoorOpen data-icon="inline-start" />邀请码加入</Button>
              <Button className="min-h-11 lg:min-h-8" onClick={onCreate}><Plus data-icon="inline-start" />创建频道</Button>
            </div>
          </header>

          {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

          {loading ? (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 3 }, (_, index) => <SpaceSkeleton key={index} />)}</div>
          ) : spaces.length === 0 ? (
            <Empty className="min-h-72 border bg-card">
              <EmptyHeader><EmptyMedia variant="icon"><RadioTower /></EmptyMedia><EmptyTitle>还没有频道</EmptyTitle><EmptyDescription>创建自己的频道，或者使用朋友发来的邀请码加入。</EmptyDescription></EmptyHeader>
            </Empty>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{spaces.map((space) => <SpaceCard key={space.id} space={space} onOpen={() => onOpenSpace(space)} />)}</div>
          )}
        </section>
      </div>
    </div>
  );
}

function LobbyLeaderboard({ entries, currentUserID, loading, error }: {
  entries: ChannelLeaderboardEntry[];
  currentUserID: number;
  loading: boolean;
  error: string;
}) {
  const ranked = [...entries].sort((left, right) => right.net_cents - left.net_cents || right.sessions - left.sessions || left.display_name.localeCompare(right.display_name));

  return (
    <Card className="overflow-hidden [--card-spacing:0px]">
      <CardContent>
        {error ? (
          <div className="p-5"><Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert></div>
        ) : loading ? (
          <LeaderboardSkeleton />
        ) : ranked.length === 0 ? (
          <Empty className="border-0 py-12"><EmptyHeader><EmptyMedia variant="icon"><Trophy /></EmptyMedia><EmptyTitle>排行榜正在等待第一场牌局</EmptyTitle><EmptyDescription>完成买入和离桌结算后，真实战绩会显示在这里。</EmptyDescription></EmptyHeader></Empty>
        ) : (
          <Table>
            <TableHeader>
              <TableRow><TableHead className="w-16 text-center">排名</TableHead><TableHead>玩家</TableHead><TableHead className="hidden text-right sm:table-cell">场次</TableHead><TableHead className="text-right">净胜</TableHead></TableRow>
            </TableHeader>
            <TableBody>
              {ranked.map((entry, index) => (
                <TableRow className={cn(entry.user_id === currentUserID && "bg-muted/50")} key={entry.user_id}>
                  <TableCell className="text-center"><Badge className="size-7 justify-center rounded-full p-0" variant={index === 0 ? "default" : index < 3 ? "secondary" : "outline"}>{index + 1}</Badge></TableCell>
                  <TableCell><div className="flex min-w-0 items-center gap-3"><div className="relative pt-1">{index === 0 && <Crown aria-hidden="true" className="absolute -top-2 left-1/2 size-3.5 -translate-x-1/2 fill-current text-winner" />}<Avatar className="size-9"><AvatarImage src={entry.avatar_url} alt={entry.display_name} /><AvatarFallback>{initials(entry.display_name)}</AvatarFallback></Avatar></div><div className="min-w-0"><strong className="block truncate">{entry.display_name}</strong>{entry.user_id === currentUserID && <span className="text-xs text-muted-foreground">这是你</span>}</div></div></TableCell>
                  <TableCell className="hidden text-right tabular-nums text-muted-foreground sm:table-cell">{entry.sessions}</TableCell>
                  <TableCell className={cn("text-right font-semibold tabular-nums", entry.net_cents > 0 && "text-success", entry.net_cents < 0 && "text-destructive", entry.net_cents === 0 && "text-muted-foreground")}>{netMoney(entry.net_cents)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function LeaderboardSkeleton() {
  return (
    <div className="flex flex-col gap-2 p-5" aria-hidden="true">
      {Array.from({ length: 4 }, (_, index) => <Skeleton className="h-14 w-full" key={index} />)}
    </div>
  );
}

function SpaceCard({ space, onOpen }: { space: Space; onOpen: () => void }) {
  return (
    <Card className="lobby-channel-card h-full [--card-spacing:--spacing(6)]">
      <CardHeader>
        <div className="flex min-w-0 items-center gap-3">
          <span className="game-room-avatar grid size-12 shrink-0 place-items-center rounded-xl font-heading text-lg font-semibold">{space.name.slice(0, 1).toUpperCase()}</span>
          <div className="min-w-0"><CardTitle className="truncate text-lg">{space.name}</CardTitle><CardDescription>{space.can_manage ? "你管理的频道" : "朋友的频道"}</CardDescription></div>
        </div>
        <CardAction><Badge variant={space.can_manage ? "secondary" : "outline"}>{space.can_manage ? "管理员" : "成员"}</Badge></CardAction>
      </CardHeader>
      <CardContent className="flex flex-1 items-end justify-between gap-4">
        <p className="text-sm leading-6 text-muted-foreground">查看牌桌、在线牌友和频道排名。</p>
        <RadioTower className="shrink-0 text-muted-foreground" />
      </CardContent>
      <CardFooter>
        <Button className="min-h-11 w-full" size="lg" onClick={onOpen}>进入频道<ArrowRight data-icon="inline-end" /></Button>
      </CardFooter>
    </Card>
  );
}

function SpaceSkeleton() {
  return <Card className="lobby-channel-card [--card-spacing:--spacing(6)]" aria-hidden="true"><CardHeader><div className="flex items-center gap-3"><Skeleton className="size-12 rounded-xl" /><div className="grid flex-1 gap-2"><Skeleton className="h-5 w-2/3" /><Skeleton className="h-3 w-1/2" /></div></div></CardHeader><CardContent><Skeleton className="h-10 w-full" /></CardContent><CardFooter><Skeleton className="h-10 w-full" /></CardFooter></Card>;
}

function SpaceDialog({ mode, onClose, onCreated }: { mode: "create" | "join"; onClose: () => void; onCreated: (space: Space) => void }) {
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [token, setToken] = useState("");
  const [invite, setInvite] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = mode === "create"
        ? await post<{ space: Space; warning?: string }>("/api/spaces", { name, newapi_base_url: baseURL, admin_token: token, quota_per_usd: 500000 })
        : await post<{ space: Space; warning?: string }>("/api/spaces/join", { invite_code: invite });
      if (result.warning) toast.warning(result.warning);
      onCreated(result.space);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>{mode === "create" ? "创建牌局频道" : "加入牌友频道"}</DialogTitle><DialogDescription>{mode === "create" ? "每个频道连接一套 New API，并自动创建独立玩家账号。" : "输入邀请码后会自动创建并绑定当前频道的 New API 玩家账号。"}</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            {mode === "create" ? (
              <>
                <Field><FieldLabel htmlFor="space-name">频道名称</FieldLabel><Input id="space-name" name="space-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：周五晚牌局…" autoComplete="off" required /></Field>
                <Field><FieldLabel htmlFor="space-url">New API 地址</FieldLabel><Input id="space-url" name="space-url" type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="例如：http://192.168.1.20:3000…" autoComplete="url" spellCheck={false} required /></Field>
                <Field><FieldLabel htmlFor="admin-token">管理员 System Access Token</FieldLabel><Input id="admin-token" name="admin-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="粘贴管理员 Token…" autoComplete="off" spellCheck={false} required /><FieldDescription>用于玩家买入和离桌时增减余额，只会加密保存。</FieldDescription></Field>
              </>
            ) : (
              <Field><FieldLabel htmlFor="invite-code">频道邀请码</FieldLabel><InputGroup><InputGroupAddon><Copy /></InputGroupAddon><InputGroupInput id="invite-code" name="invite-code" className="font-mono uppercase" value={invite} onChange={(event) => setInvite(event.target.value.toUpperCase())} placeholder="例如：A1B2C3D4…" autoComplete="off" spellCheck={false} required /></InputGroup></Field>
            )}
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在验证…" : mode === "create" ? "验证并创建" : "加入频道"}{!busy && <ArrowRight data-icon="inline-end" />}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function initials(name: string) {
  return name.trim().slice(0, 2).toUpperCase();
}

function roleLabel(role: User["role"]) {
  return ({ super_admin: "超级管理员", operator: "运营", player: "玩家" } as const)[role];
}

function netMoney(cents: number) {
  const sign = cents > 0 ? "+" : cents < 0 ? "−" : "";
  return `${sign}$${(Math.abs(cents) / 100).toFixed(2)}`;
}
