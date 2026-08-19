import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowRight, CheckCircle2, Copy, DoorOpen, Gamepad2, KeyRound, LogOut,
  Menu, Plus, Server, Settings2, ShieldCheck,
} from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
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
import { BrandMark } from "@/components/brand-mark";
import { WeChatIcon } from "@/components/wechat-icon";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { api, post } from "./api";
import type { Space, User } from "./types";

interface Props {
  user: User;
  wechatLoginEnabled: boolean;
  onOpenSpace: (space: Space) => void;
  onOpenAdmin?: () => void;
  onLogout: () => void;
}

export default function Dashboard({ user, wechatLoginEnabled, onOpenSpace, onOpenAdmin, onLogout }: Props) {
  const [spaces, setSpaces] = useState<Space[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialog, setDialog] = useState<"create" | "join" | null>(null);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api<{ spaces: Space[] }>("/api/spaces")
      .then((result) => setSpaces(result.spaces || []))
      .catch((caught) => setError(caught instanceof Error ? caught.message : "读取频道失败"))
      .finally(() => setLoading(false));
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
        onOpenMenu={() => setMobileOpen(true)}
        onOpenAdmin={onOpenAdmin}
        wechatLoginEnabled={wechatLoginEnabled}
        onLogout={() => void logout()}
      />

      <div className="flex min-h-0 flex-1">
        <main id="main-content" className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <LobbyView user={user} spaces={spaces} loading={loading} error={error} onCreate={() => setDialog("create")} onJoin={() => setDialog("join")} onOpenSpace={openSpace} />
        </main>
      </div>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-72 p-0" showCloseButton={false}>
          <SheetHeader className="border-b"><SheetTitle>游戏大厅</SheetTitle><SheetDescription>选择功能或频道</SheetDescription></SheetHeader>
          <LobbySidebar
            className="flex min-h-0 flex-1 border-0"
            spaces={spaces}
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

function GameHeader({ user, onOpenMenu, onOpenAdmin, wechatLoginEnabled, onLogout }: {
  user: User;
  onOpenMenu: () => void;
  onOpenAdmin?: () => void;
  wechatLoginEnabled: boolean;
  onLogout: () => void;
}) {
  return (
    <header className="game-topbar flex h-16 shrink-0 items-center justify-between gap-4 px-3 sm:px-6">
      <div className="flex min-w-0 items-center gap-2 sm:gap-4">
        <Button size="icon" variant="secondary" className="md:hidden" onClick={onOpenMenu} aria-label="打开游戏导航"><Menu /></Button>
        <div className="flex min-w-0 items-center gap-3">
          <BrandMark className="size-10" aria-hidden="true" />
          <span className="hidden min-w-0 sm:block"><strong className="block truncate font-heading">PokerNode</strong><small className="block truncate">Texas Hold’em Hall</small></span>
        </div>
      </div>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="secondary" className="h-auto gap-2 p-1.5 pr-2"><Avatar className="size-8"><AvatarFallback>{initials(user.display_name)}</AvatarFallback></Avatar><span className="hidden min-w-0 text-left sm:block"><strong className="block max-w-32 truncate text-xs">{user.display_name}</strong><small className="block text-xs text-muted-foreground">{roleLabel(user.role)}</small></span></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56"><DropdownMenuLabel><span className="block text-foreground">{user.display_name}</span><span className="block font-normal">@{user.username}</span></DropdownMenuLabel><DropdownMenuSeparator /><DropdownMenuGroup>{onOpenAdmin && <DropdownMenuItem onSelect={onOpenAdmin}><ShieldCheck />打开运营后台</DropdownMenuItem>}{wechatLoginEnabled && (user.wechat_bound ? <DropdownMenuItem disabled><CheckCircle2 />微信已绑定</DropdownMenuItem> : <DropdownMenuItem onSelect={() => window.location.assign("/api/auth/wechat/link")}><WeChatIcon />绑定微信</DropdownMenuItem>)}<DropdownMenuItem variant="destructive" onSelect={onLogout}><LogOut />退出登录</DropdownMenuItem></DropdownMenuGroup></DropdownMenuContent>
      </DropdownMenu>
    </header>
  );
}

function LobbySidebar({ className, spaces, onOpenSpace, onCreate, onJoin }: {
  className?: string;
  spaces: Space[];
  onOpenSpace: (space: Space) => void;
  onCreate: () => void;
  onJoin: () => void;
}) {
  return (
    <aside className={cn("min-h-0 flex-col border-r bg-card", className)}>
      <div className="flex flex-col gap-2 p-4">
        <p className="text-xs font-medium text-muted-foreground">快速开始</p>
        <div className="grid grid-cols-2 gap-2"><Button onClick={onCreate}><Plus data-icon="inline-start" />创建</Button><Button variant="outline" onClick={onJoin}><DoorOpen data-icon="inline-start" />加入</Button></div>
      </div>
      <Separator />
      <div className="flex flex-col gap-1 p-3">
        <Button className="justify-start" variant="secondary"><Gamepad2 data-icon="inline-start" />游戏大厅</Button>
      </div>
      <Separator />
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3"><LobbySidebarGroup label="你的牌局" spaces={spaces} empty="还没有牌局" onOpenSpace={onOpenSpace} /></div>
      </ScrollArea>
      <Separator />
      <div className="p-4"><div className="flex items-center gap-3 text-xs text-muted-foreground"><span className="relative flex size-2"><span className="absolute inline-flex size-full animate-ping rounded-full bg-primary opacity-50 motion-reduce:animate-none" /><span className="relative inline-flex size-2 rounded-full bg-primary" /></span><span>服务运行正常</span></div></div>
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
        <Button key={space.id} className="h-auto justify-start py-2" variant="ghost" onClick={() => onOpenSpace(space)}><span className="game-room-avatar grid size-8 shrink-0 place-items-center rounded-lg font-heading font-semibold">{space.name.slice(0, 1).toUpperCase()}</span><span className="min-w-0 flex-1 text-left"><strong className="block truncate text-sm">{space.name}</strong><small className="block truncate text-xs text-muted-foreground">{space.can_manage ? "我管理的牌局" : "已加入的牌局"}</small></span></Button>
      ))}
    </div>
  );
}

function LobbyView({ user, spaces, loading, error, onCreate, onJoin, onOpenSpace }: {
  user: User;
  spaces: Space[];
  loading: boolean;
  error: string;
  onCreate: () => void;
  onJoin: () => void;
  onOpenSpace: (space: Space) => void;
}) {
  return (
    <div className="player-lobby flex flex-1 overflow-auto">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-10 px-4 py-8 sm:px-6 sm:py-10 lg:px-8">
        <header className="flex max-w-2xl flex-col gap-3" aria-labelledby="lobby-title">
          <Badge variant="outline" className="w-fit"><Gamepad2 data-icon="inline-start" />游戏大厅</Badge>
          <h1 id="lobby-title" className="font-heading text-3xl font-semibold tracking-tight sm:text-4xl">欢迎回来，{user.display_name}</h1>
          <p className="text-base leading-7 text-muted-foreground">想自己开一桌，还是加入朋友的牌局？选择一种方式就可以开始。</p>
        </header>

        {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

        <section className="flex flex-col gap-4" aria-labelledby="quick-start-title">
          <div>
            <h2 id="quick-start-title" className="font-heading text-xl font-semibold">开始一场牌局</h2>
            <p className="mt-1 text-sm text-muted-foreground">选择适合你的方式，几步即可进入牌桌。</p>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <QuickStartCard mode="create" onAction={onCreate} />
            <QuickStartCard mode="join" onAction={onJoin} />
          </div>
        </section>

        <section className="flex flex-col gap-4" aria-labelledby="your-games-title">
          <div className="flex items-end justify-between gap-4">
            <div>
              <div className="flex items-center gap-2"><h2 id="your-games-title" className="font-heading text-xl font-semibold">你的牌局</h2><Badge variant="outline">{spaces.length}</Badge></div>
              <p className="mt-1 text-sm text-muted-foreground">你创建和加入过的牌局都会集中在这里。</p>
            </div>
          </div>

          {loading ? (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 3 }, (_, index) => <SpaceSkeleton key={index} />)}</div>
          ) : spaces.length === 0 ? (
            <Empty className="min-h-64 border bg-card">
              <EmptyHeader><EmptyMedia variant="icon"><Gamepad2 /></EmptyMedia><EmptyTitle>还没有牌局</EmptyTitle><EmptyDescription>从上面选择“发起牌局”或“输入邀请码”，完成后会显示在这里。</EmptyDescription></EmptyHeader>
            </Empty>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{spaces.map((space) => <SpaceCard key={space.id} space={space} onOpen={() => onOpenSpace(space)} />)}</div>
          )}
        </section>
      </div>
    </div>
  );
}

function QuickStartCard({ mode, onAction }: { mode: "create" | "join"; onAction: () => void }) {
  const create = mode === "create";
  return (
    <Card>
      <CardHeader>
        <CardTitle>{create ? "发起新牌局" : "加入朋友的牌局"}</CardTitle>
        <CardDescription>{create ? "创建一个专属牌局，设置好牌桌后把邀请码发给朋友。" : "输入朋友发来的邀请码，进入他们已经准备好的牌局。"}</CardDescription>
        <CardAction><Badge variant={create ? "secondary" : "outline"}>{create ? "我是房主" : "我有邀请码"}</Badge></CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-sm text-muted-foreground">
        {create ? (
          <><p className="flex items-center gap-2"><ShieldCheck className="size-4 shrink-0" />你负责牌桌、邀请和成员设置</p><p className="flex items-center gap-2"><Copy className="size-4 shrink-0" />创建后把专属邀请码发给朋友</p></>
        ) : (
          <><p className="flex items-center gap-2"><DoorOpen className="size-4 shrink-0" />只需要朋友分享的邀请码</p><p className="flex items-center gap-2"><Gamepad2 className="size-4 shrink-0" />加入后即可选择牌桌开始游戏</p></>
        )}
      </CardContent>
      <CardFooter>
        <Button className="w-full" size="lg" variant={create ? "default" : "outline"} onClick={onAction}>
          {create ? <Plus data-icon="inline-start" /> : <DoorOpen data-icon="inline-start" />}
          {create ? "发起牌局" : "输入邀请码"}
          <ArrowRight data-icon="inline-end" />
        </Button>
      </CardFooter>
    </Card>
  );
}

function SpaceCard({ space, onOpen }: { space: Space; onOpen: () => void }) {
  return (
    <Card size="sm" className="lobby-channel-card">
      <CardHeader>
        <div className="flex min-w-0 items-center gap-3">
          <span className="game-room-avatar grid size-10 shrink-0 place-items-center rounded-xl font-heading font-semibold">{space.name.slice(0, 1).toUpperCase()}</span>
          <div className="min-w-0"><CardTitle className="truncate">{space.name}</CardTitle><CardDescription className="truncate">{hostOf(space.newapi_base_url)}</CardDescription></div>
        </div>
        <CardAction><Badge variant="outline">{space.can_manage ? "我管理" : "已加入"}</Badge></CardAction>
      </CardHeader>
      <CardContent>
        <div className="lobby-channel-meta">
          <span><KeyRound />{space.is_bound ? "个人凭证已绑定" : "等待绑定个人凭证"}</span>
          <span><Server />独立 New API 连接</span>
        </div>
      </CardContent>
      <CardFooter className="lobby-channel-card__footer">
        <span>{space.can_manage ? "可管理牌桌与邀请" : "可以继续进入牌桌"}</span>
        <Button className="w-full sm:w-auto" size="sm" variant={space.can_manage ? "secondary" : "default"} onClick={onOpen}>{space.can_manage ? <Settings2 data-icon="inline-start" /> : <ArrowRight data-icon="inline-start" />}{space.can_manage ? "管理牌局" : "进入牌局"}</Button>
      </CardFooter>
    </Card>
  );
}

function SpaceSkeleton() {
  return <Card size="sm" className="lobby-channel-card" aria-hidden="true"><CardHeader><div className="flex items-center gap-3"><Skeleton className="size-10 rounded-xl" /><div className="grid flex-1 gap-2"><Skeleton className="h-4 w-2/3" /><Skeleton className="h-3 w-1/2" /></div></div></CardHeader><CardContent><Skeleton className="h-8 w-full" /></CardContent><CardFooter className="justify-between gap-3"><Skeleton className="h-3 w-1/2" /><Skeleton className="h-7 w-24" /></CardFooter></Card>;
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

function hostOf(value: string) {
  try { return new URL(value).host; } catch { return value; }
}
