import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Activity, Eye, EyeOff, KeyRound, LockKeyhole, Plus, RadioTower, Search, Server, ShieldAlert, ShieldCheck, Table2, Trash2, Trophy, UserCog, UsersRound } from "lucide-react";
import { toast } from "sonner";
import { ChartAreaInteractive } from "@/components/chart-area-interactive";
import { SectionCards } from "@/components/section-cards";
import type { AdminSection } from "@/components/app-sidebar";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldContent, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, patch, post, put, remove } from "./api";
import BalanceManager from "./BalanceManager";
import RoleManager from "./RoleManager";
import type { AdminOverview, AdminSpaceSummary, Role, User, UserRole, UserStatus } from "./types";

interface Props {
  currentUser: User;
  section: AdminSection;
  onRegistrationChanged: (enabled: boolean) => void;
}

function normalizeAdminOverview(result: AdminOverview): AdminOverview {
  return {
    ...result,
    users: Array.isArray(result.users) ? result.users : [],
    counts: result.counts ?? {},
    spaces: Array.isArray(result.spaces) ? result.spaces : [],
    platform_counts: result.platform_counts ?? {
      spaces: 0,
      memberships: 0,
      bound_memberships: 0,
      tables: 0,
      operations: 0,
      failed_operations: 0,
    },
    permissions: Array.isArray(result.permissions) ? result.permissions : [],
    roles: Array.isArray(result.roles)
      ? result.roles.map((role) => ({
          ...role,
          permissions: Array.isArray(role.permissions) ? role.permissions : [],
        }))
      : [],
    permission_catalog: Array.isArray(result.permission_catalog) ? result.permission_catalog : [],
  };
}

export default function AdminConsole({ currentUser, section, onRegistrationChanged }: Props) {
  const [overview, setOverview] = useState<AdminOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [deleting, setDeleting] = useState<User | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [confirmCloseRegistration, setConfirmCloseRegistration] = useState(false);
  const [registrationBusy, setRegistrationBusy] = useState(false);

  const load = async () => {
    setError("");
    try {
      const result = await api<AdminOverview>("/api/admin/overview");
      const normalized = normalizeAdminOverview(result);
      setOverview(normalized);
      onRegistrationChanged(normalized.registration_enabled);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "读取运营数据失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const users = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!overview || !needle) return overview?.users || [];
    return overview.users.filter((user) => `${user.username} ${user.display_name} ${user.role_name || roleLabel(user.role, overview.roles)}`.toLowerCase().includes(needle));
  }, [overview, query]);

  const permissions = new Set(overview?.permissions || []);
  const canManageUsers = permissions.has("users:manage");
  const canManageRoles = permissions.has("roles:manage");
  const canManageRegistration = permissions.has("registration:manage");

  async function saveRegistration(enabled: boolean) {
    setRegistrationBusy(true);
    try {
      await put("/api/admin/settings/registration", { enabled });
      setOverview((current) => current ? { ...current, registration_enabled: enabled } : current);
      onRegistrationChanged(enabled);
      toast.success(enabled ? "已开启自助注册" : "已关闭自助注册");
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "注册设置保存失败");
    } finally {
      setRegistrationBusy(false);
      setConfirmCloseRegistration(false);
    }
  }

  async function deleteUser() {
    if (!deleting) return;
    setDeleteBusy(true);
    try {
      await remove(`/api/admin/users/${deleting.id}`);
      setOverview((current) => current ? withoutUser(current, deleting.id) : current);
      toast.success(`已删除账号 @${deleting.username}`);
      setDeleting(null);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "删除账号失败");
    } finally {
      setDeleteBusy(false);
    }
  }

  if (loading) return <AdminLoading />;

  if (error || !overview) {
    return <div className="p-4 sm:p-6 lg:p-8"><Alert variant="destructive"><AlertDescription>{error || "运营数据不可用"}</AlertDescription></Alert></div>;
  }

  if (section === "overview") {
    const bindingHealthy = overview.platform_counts.memberships === overview.platform_counts.bound_memberships;
    return (
      <div className="flex flex-1 flex-col gap-6 overflow-auto bg-background p-4 sm:p-6 lg:p-8">
        <SectionCards overview={overview} />
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.5fr)]">
          <ChartAreaInteractive spaces={overview.spaces} />
          <Card>
            <CardHeader><CardTitle>平台运行状态</CardTitle><CardDescription>根据当前数据库记录汇总，不包含模拟数据。</CardDescription></CardHeader>
            <CardContent className="grid gap-4">
              <HealthRow icon={<Server />} title="后台 API" value="在线" healthy />
              <HealthRow icon={<KeyRound />} title="成员凭证" value={bindingHealthy ? "全部已绑定" : `${overview.platform_counts.memberships - overview.platform_counts.bound_memberships} 个待绑定`} healthy={bindingHealthy} />
              <HealthRow icon={<Activity />} title="结算流水" value={overview.platform_counts.failed_operations ? `${overview.platform_counts.failed_operations} 条失败` : "未发现失败"} healthy={overview.platform_counts.failed_operations === 0} />
              <HealthRow icon={<Table2 />} title="持久化牌桌" value={`${overview.platform_counts.tables} 桌`} healthy />
            </CardContent>
          </Card>
        </div>
        <Card>
          <CardHeader><CardTitle>最近频道</CardTitle><CardDescription>快速核对 New API 节点、负责人和当前规模。</CardDescription></CardHeader>
          <CardContent><ChannelTable spaces={overview.spaces.slice(0, 5)} compact /></CardContent>
        </Card>
      </div>
    );
  }

  const sectionAllowed = {
    users: permissions.has("users:read") || canManageUsers,
    roles: canManageRoles,
    channels: permissions.has("channels:manage"),
    balances: permissions.has("balances:manage"),
    rankings: currentUser.role === "super_admin",
    settings: canManageRegistration,
  }[section];
  if (!sectionAllowed) {
    return <div className="p-4 sm:p-6 lg:p-8"><Alert variant="destructive"><AlertDescription>当前角色没有访问此功能的权限。</AlertDescription></Alert></div>;
  }

  if (section === "roles") {
    return <RoleManager roles={overview.roles} catalog={overview.permission_catalog} onChanged={(roles) => setOverview((current) => current ? { ...current, roles } : current)} />;
  }

  if (section === "rankings") {
    return <RankingManager users={users} allUsers={overview.users} query={query} onQueryChange={setQuery} onChanged={(user) => setOverview((current) => current ? withUser(current, user) : current)} />;
  }

  if (section === "channels") {
    return (
      <div className="flex flex-1 flex-col gap-6 overflow-auto bg-background p-4 sm:p-6 lg:p-8">
        <div className="grid gap-4 sm:grid-cols-3">
          <SummaryMetric title="频道总数" value={overview.platform_counts.spaces} description="独立 New API 节点" icon={<RadioTower />} />
          <SummaryMetric title="频道成员" value={overview.platform_counts.memberships} description={`${overview.platform_counts.bound_memberships} 个已绑定`} icon={<UsersRound />} />
          <SummaryMetric title="牌桌总数" value={overview.platform_counts.tables} description="数据库持久化牌桌" icon={<Table2 />} />
        </div>
        <Card>
          <CardHeader><CardTitle>频道节点</CardTitle><CardDescription>管理员 Token 仅显示末四位；密文不会返回前端。</CardDescription><CardAction><Badge variant="outline">{overview.spaces.length} 个节点</Badge></CardAction></CardHeader>
          <CardContent><ChannelTable spaces={overview.spaces} /></CardContent>
        </Card>
      </div>
    );
  }

  if (section === "balances") {
    return <BalanceManager spaces={overview.spaces.map((space) => ({
      id: space.id,
      name: space.name,
      member_count: space.member_count,
      bound_member_count: space.bound_member_count,
      newapi_base_url: space.newapi_base_url,
    }))} />;
  }

  if (section === "settings") {
    return (
      <div className="flex flex-1 flex-col gap-6 overflow-auto bg-background p-4 sm:p-6 lg:p-8">
        <Card>
          <CardHeader><CardTitle>注册策略</CardTitle><CardDescription>控制玩家登录页是否允许访客自行创建 PokerNode 账号。</CardDescription><CardAction><Badge variant={overview.registration_enabled ? "secondary" : "outline"}>{overview.registration_enabled ? "开放" : "已关闭"}</Badge></CardAction></CardHeader>
          <CardContent>
            <Field orientation="horizontal" data-disabled={!canManageRegistration}>
              <FieldContent><FieldTitle>允许自助注册</FieldTitle><FieldDescription>关闭后注册入口和注册 API 同时停用；后台仍可创建账号。</FieldDescription></FieldContent>
              <Switch checked={overview.registration_enabled} disabled={!canManageRegistration || registrationBusy} aria-label="允许自助注册" onCheckedChange={(checked) => checked ? void saveRegistration(true) : setConfirmCloseRegistration(true)} />
            </Field>
            {!canManageRegistration && <Alert className="mt-4"><AlertDescription>当前角色没有修改平台注册策略的权限。</AlertDescription></Alert>}
          </CardContent>
        </Card>
        <AlertDialog open={confirmCloseRegistration} onOpenChange={setConfirmCloseRegistration}>
          <AlertDialogContent><AlertDialogHeader><AlertDialogMedia><ShieldAlert /></AlertDialogMedia><AlertDialogTitle>关闭自助注册？</AlertDialogTitle><AlertDialogDescription>现有账号不受影响；访客将无法创建新账号。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction onClick={() => void saveRegistration(false)}>确认关闭</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
        </AlertDialog>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col gap-6 overflow-auto bg-background p-4 sm:p-6 lg:p-8">
      <section className="grid overflow-hidden rounded-xl border bg-card sm:grid-cols-2 xl:grid-cols-4">
        <SummaryMetric title="全部用户" value={overview.counts.total || 0} description="PokerNode 独立账号" icon={<UsersRound />} />
        <SummaryMetric title="活跃账号" value={overview.counts.active || 0} description="当前允许登录" icon={<ShieldCheck />} />
        <SummaryMetric title="频道管理员" value={overview.users.filter((user) => (user.managed_space_ids?.length || 0) > 0).length} description="可同时负责多个频道" icon={<UserCog />} />
        <SummaryMetric title="自定义角色" value={overview.roles.filter((role) => !role.system).length} description="功能权限可自定义" icon={<LockKeyhole />} />
      </section>

      <Card>
        <CardHeader>
          <CardTitle>用户与权限</CardTitle>
          <CardDescription>角色决定功能权限；频道管理角色用户可单独分配一个或多个频道。</CardDescription>
          <CardAction><Button disabled={!canManageUsers} onClick={() => setCreateOpen(true)}><Plus data-icon="inline-start" />创建账号</Button></CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <InputGroup className="max-w-sm">
            <InputGroupAddon><Search /></InputGroupAddon>
            <InputGroupInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索用户名、显示名称或角色" aria-label="搜索用户" />
          </InputGroup>

          {users.length === 0 ? (
            <Empty className="min-h-56">
              <EmptyHeader><EmptyMedia variant="icon"><UsersRound /></EmptyMedia><EmptyTitle>没有匹配的用户</EmptyTitle><EmptyDescription>换一个关键词，或创建新的平台账号。</EmptyDescription></EmptyHeader>
            </Empty>
          ) : (
            <Table>
              <TableHeader><TableRow><TableHead>用户</TableHead><TableHead>平台角色</TableHead><TableHead>状态</TableHead><TableHead>创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {users.map((user) => {
                  const canEdit = canManageUsers && user.id !== currentUser.id && (canManageRoles || user.role === "player");
                  return (
                    <TableRow key={user.id}>
                      <TableCell><div className="flex items-center gap-3"><Avatar><AvatarFallback>{initials(user.display_name)}</AvatarFallback></Avatar><span><strong className="block">{user.display_name}</strong><small className="text-muted-foreground">@{user.username}</small></span></div></TableCell>
                      <TableCell><Badge variant={user.role === "super_admin" ? "default" : user.role === "channel_manager" ? "secondary" : "outline"}>{user.role_name || roleLabel(user.role, overview.roles)}</Badge><small className="mt-1 block text-muted-foreground">已加入 {user.joined_space_ids?.length || 0} 个 · 管理 {user.managed_space_ids?.length || 0} 个</small></TableCell>
                      <TableCell><Badge variant={user.status === "active" ? "secondary" : "destructive"}>{user.status === "active" ? "正常" : "已停用"}</Badge></TableCell>
                      <TableCell className="text-muted-foreground">{new Date(user.created_at).toLocaleDateString()}</TableCell>
                      <TableCell className="text-right"><Button size="sm" variant="outline" disabled={!canEdit} onClick={() => setEditing(user)}>{user.id === currentUser.id ? "当前账号" : "管理"}</Button></TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
        <CardFooter className="justify-between text-xs text-muted-foreground"><span>共 {overview.users.length} 个用户</span><span>角色控制功能，频道范围支持多选</span></CardFooter>
      </Card>

      <CreateUserDialog open={createOpen} canManageRoles={canManageRoles} roles={overview.roles} onClose={() => setCreateOpen(false)} onCreated={(user) => {
        setCreateOpen(false);
        setOverview((current) => current ? withUser(current, user) : current);
        toast.success("账号已创建");
      }} />
      {editing && <EditUserDialog user={editing} canManageRoles={canManageRoles} roles={overview.roles} spaces={overview.spaces} onClose={() => setEditing(null)} onDeleteRequested={(user) => {
        setEditing(null);
        setDeleting(user);
      }} onSaved={(user) => {
        setEditing(null);
        setOverview((current) => current ? withUser(current, user) : current);
        toast.success("账号资料和权限已更新");
      }} />}

      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && !deleteBusy && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia><Trash2 /></AlertDialogMedia>
            <AlertDialogTitle>永久删除账号？</AlertDialogTitle>
            <AlertDialogDescription>将删除 {deleting?.display_name}（@{deleting?.username}）及其频道成员关系。拥有频道或钱包流水的账号会被系统拒绝删除，请改为停用。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel disabled={deleteBusy}>取消</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={deleteBusy} onClick={(event) => { event.preventDefault(); void deleteUser(); }}>{deleteBusy && <Spinner data-icon="inline-start" />}{deleteBusy ? "正在删除…" : "确认删除"}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

    </div>
  );
}

function HealthRow({ icon, title, value, healthy }: { icon: React.ReactNode; title: string; value: string; healthy: boolean }) {
  return <div className="flex items-center gap-3 rounded-lg border p-3"><span className="grid size-9 place-items-center rounded-lg bg-muted text-muted-foreground">{icon}</span><span className="min-w-0 flex-1"><strong className="block text-sm">{title}</strong><small className="block truncate text-muted-foreground">{value}</small></span><span className={healthy ? "size-2 rounded-full bg-emerald-500" : "size-2 rounded-full bg-destructive"} aria-label={healthy ? "正常" : "需处理"} /></div>;
}

function ChannelTable({ spaces, compact = false }: { spaces: AdminSpaceSummary[]; compact?: boolean }) {
  if (spaces.length === 0) return <Empty className="min-h-48"><EmptyHeader><EmptyMedia variant="icon"><RadioTower /></EmptyMedia><EmptyTitle>还没有频道</EmptyTitle><EmptyDescription>玩家创建频道并连接 New API 后，会在这里显示。</EmptyDescription></EmptyHeader></Empty>;
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader><TableRow><TableHead>频道 / New API</TableHead><TableHead>负责人</TableHead><TableHead>成员绑定</TableHead><TableHead>牌桌</TableHead>{!compact && <TableHead>资金流水</TableHead>}<TableHead>状态</TableHead></TableRow></TableHeader>
        <TableBody>{spaces.map((space) => <TableRow key={space.id}><TableCell><strong className="block">{space.name}</strong><small className="block max-w-64 truncate text-muted-foreground">{hostOf(space.newapi_base_url)} · Token …{space.admin_token_last4}</small></TableCell><TableCell><span className="block">{space.owner_display_name}</span><small className="text-muted-foreground">@{space.owner_username}</small></TableCell><TableCell className="tabular-nums">{space.bound_member_count} / {space.member_count}</TableCell><TableCell className="tabular-nums">{space.table_count}</TableCell>{!compact && <TableCell className="tabular-nums">{space.operation_count}</TableCell>}<TableCell><Badge variant={space.failed_operations ? "destructive" : space.bound_member_count < space.member_count ? "outline" : "secondary"}>{space.failed_operations ? `${space.failed_operations} 条失败` : space.bound_member_count < space.member_count ? "待绑定" : "正常"}</Badge></TableCell></TableRow>)}</TableBody>
      </Table>
    </div>
  );
}

function hostOf(value: string) {
  try { return new URL(value).host; } catch { return value; }
}

function SummaryMetric({ title, value, description, icon }: { title: string; value: number; description: string; icon: React.ReactNode }) {
  return <div className="flex min-w-0 items-center gap-4 border-t p-5 first:border-t-0 sm:[&:nth-child(2)]:border-t-0 sm:[&:nth-child(even)]:border-l xl:border-t-0 xl:border-l xl:first:border-l-0"><span className="grid size-10 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">{icon}</span><span className="min-w-0"><span className="block text-xs text-muted-foreground">{title}</span><strong className="block font-heading text-2xl">{value}</strong><small className="block truncate text-muted-foreground">{description}</small></span></div>;
}

function CreateUserDialog({ open, canManageRoles, roles, onClose, onCreated }: { open: boolean; canManageRoles: boolean; roles: Role[]; onClose: () => void; onCreated: (user: User) => void }) {
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<UserRole>("player");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await post<{ user: User }>("/api/admin/users", { username, display_name: displayName, password, role, managed_space_ids: [] });
      onCreated(result.user);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "创建账号失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[min(90vh,780px)] overflow-y-auto sm:max-w-xl">
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>创建平台账号</DialogTitle><DialogDescription>账号可以直接登录 PokerNode，不依赖 New API 用户系统。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <Field><FieldLabel htmlFor="new-username">用户名</FieldLabel><Input id="new-username" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="3–24 位字母、数字或下划线" required autoFocus /></Field>
            <Field><FieldLabel htmlFor="new-display-name">显示名称</FieldLabel><Input id="new-display-name" value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="牌桌和后台显示的名称" /></Field>
            <Field><FieldLabel htmlFor="new-password">初始密码</FieldLabel><Input id="new-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="至少 8 位" required /></Field>
            <Field data-disabled={!canManageRoles}><FieldLabel>平台角色</FieldLabel><RoleSelect value={role} roles={roles} disabled={!canManageRoles} onChange={setRole} /><FieldDescription>{canManageRoles ? "角色决定这个账号可以使用哪些功能。" : "当前账号只能创建玩家。"}</FieldDescription></Field>
            {role === "super_admin" ? <Alert><AlertDescription>超级管理员自动拥有全部权限和全部频道，不需要单独分配。</AlertDescription></Alert> : roleUsesChannelScope(roles, role) ? <Alert><AlertDescription>新账号还没有加入频道。创建后请让用户先加入频道，再回到用户管理中分配其管理范围。</AlertDescription></Alert> : null}
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在创建…" : "创建账号"}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function EditUserDialog({ user, canManageRoles, roles, spaces, onClose, onDeleteRequested, onSaved }: { user: User; canManageRoles: boolean; roles: Role[]; spaces: AdminSpaceSummary[]; onClose: () => void; onDeleteRequested: (user: User) => void; onSaved: (user: User) => void }) {
  const joinedSpaceIDs = user.joined_space_ids || [];
  const joinedSpaces = spaces.filter((space) => joinedSpaceIDs.includes(space.id));
  const [username, setUsername] = useState(user.username);
  const [displayName, setDisplayName] = useState(user.display_name);
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<UserRole>(user.role);
  const [managedSpaceIDs, setManagedSpaceIDs] = useState<string[]>((user.managed_space_ids || []).filter((spaceID) => joinedSpaceIDs.includes(spaceID)));
  const [status, setStatus] = useState<UserStatus>(user.status);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await patch<{ user: User }>(`/api/admin/users/${user.id}`, { username, display_name: displayName, password, role, status, managed_space_ids: managedSpaceIDs });
      onSaved(result.user);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "更新用户失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[min(90vh,780px)] overflow-y-auto sm:max-w-xl">
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>管理 {user.display_name}</DialogTitle><DialogDescription>@{user.username} · 在这里设置角色及其可管理的频道范围。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <Field><FieldLabel htmlFor={`edit-username-${user.id}`}>用户名</FieldLabel><Input id={`edit-username-${user.id}`} value={username} onChange={(event) => setUsername(event.target.value)} placeholder="3–24 位字母、数字或下划线" required autoFocus /></Field>
            <Field><FieldLabel htmlFor={`edit-display-name-${user.id}`}>显示名称</FieldLabel><Input id={`edit-display-name-${user.id}`} value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="牌桌和后台显示的名称" required /></Field>
            <Field><FieldLabel htmlFor={`edit-password-${user.id}`}>重置密码</FieldLabel><Input id={`edit-password-${user.id}`} type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="留空则保持原密码" autoComplete="new-password" /><FieldDescription>如需重置，输入 8–72 位新密码。</FieldDescription></Field>
            <Field data-disabled={!canManageRoles}><FieldLabel>平台角色</FieldLabel><RoleSelect value={role} roles={roles} disabled={!canManageRoles} onChange={(next) => { setRole(next); if (!roleUsesChannelScope(roles, next)) setManagedSpaceIDs([]); }} /><FieldDescription>角色决定功能权限；频道范围在下方独立设置。</FieldDescription></Field>
            {role === "super_admin" ? <Alert><AlertDescription>超级管理员自动拥有全部权限和全部频道，不需要单独分配。</AlertDescription></Alert> : roleUsesChannelScope(roles, role) ? <ChannelScopeField spaces={joinedSpaces} selected={managedSpaceIDs} onChange={setManagedSpaceIDs} /> : null}
            <Field><FieldLabel>账号状态</FieldLabel><Select value={status} onValueChange={(value) => setStatus(value as UserStatus)}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent position="popper"><SelectGroup><SelectItem value="active">正常</SelectItem><SelectItem value="disabled">停用</SelectItem></SelectGroup></SelectContent></Select><FieldDescription>停用后，该用户的现有会话和后续登录都会被拒绝。</FieldDescription></Field>
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6 sm:justify-between"><Button type="button" variant="destructive" disabled={busy} onClick={() => onDeleteRequested(user)}><Trash2 data-icon="inline-start" />删除账号</Button><span className="flex gap-2"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在保存…" : "保存账号"}</Button></span></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function RoleSelect({ value, roles, disabled, onChange }: { value: UserRole; roles: Role[]; disabled: boolean; onChange: (value: UserRole) => void }) {
  return <Select value={value} disabled={disabled} onValueChange={(next) => onChange(next as UserRole)}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent position="popper"><SelectGroup>{roles.map((role) => <SelectItem key={role.key} value={role.key}>{role.name}</SelectItem>)}</SelectGroup></SelectContent></Select>;
}

function ChannelScopeField({ spaces, selected, onChange }: { spaces: AdminSpaceSummary[]; selected: string[]; onChange: (ids: string[]) => void }) {
  const [query, setQuery] = useState("");
  const needle = query.trim().toLowerCase();
  const visible = needle ? spaces.filter((space) => `${space.name} ${space.owner_username} ${hostOf(space.newapi_base_url)}`.toLowerCase().includes(needle)) : spaces;
  const visibleIDs = visible.map((space) => space.id);
  const allVisibleSelected = visibleIDs.length > 0 && visibleIDs.every((id) => selected.includes(id));

  function toggleAll() {
    if (allVisibleSelected) {
      onChange(selected.filter((id) => !visibleIDs.includes(id)));
    } else {
      onChange(Array.from(new Set([...selected, ...visibleIDs])));
    }
  }

  return (
    <Field>
      <div className="flex items-end justify-between gap-3"><span><FieldLabel htmlFor="channel-scope-search">管理频道范围</FieldLabel><FieldDescription>只显示该用户已经加入的频道，可多选。</FieldDescription></span><Badge variant="secondary">已选 {selected.length}</Badge></div>
      <Input id="channel-scope-search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索频道、负责人或节点地址" />
      <div className="flex items-center justify-between"><small className="text-muted-foreground">{visible.length} 个匹配频道</small><span className="flex gap-2"><Button type="button" size="xs" variant="ghost" disabled={!visible.length} onClick={toggleAll}>{allVisibleSelected ? "取消当前结果" : "全选当前结果"}</Button>{selected.length > 0 && <Button type="button" size="xs" variant="ghost" onClick={() => onChange([])}>清空</Button>}</span></div>
      <div className="max-h-56 overflow-y-auto rounded-xl border p-2">
        {visible.length === 0 ? <p className="p-6 text-center text-sm text-muted-foreground">{spaces.length === 0 ? "该用户尚未加入任何频道" : "没有匹配的已加入频道"}</p> : <FieldGroup className="gap-1">{visible.map((space) => <Field key={space.id} orientation="horizontal" className="rounded-lg p-2 hover:bg-muted/50"><FieldContent><FieldTitle>{space.name}</FieldTitle><FieldDescription>@{space.owner_username} · {hostOf(space.newapi_base_url)}</FieldDescription></FieldContent><Switch checked={selected.includes(space.id)} onCheckedChange={(checked) => onChange(checked ? [...selected, space.id] : selected.filter((id) => id !== space.id))} aria-label={`分配频道 ${space.name}`} /></Field>)}</FieldGroup>}
      </div>
    </Field>
  );
}

function RankingManager({ users, allUsers, query, onQueryChange, onChanged }: {
  users: User[];
  allUsers: User[];
  query: string;
  onQueryChange: (value: string) => void;
  onChanged: (user: User) => void;
}) {
  const [busyID, setBusyID] = useState<number | null>(null);
  const hiddenCount = allUsers.filter((user) => user.ranking_hidden).length;

  async function toggle(user: User) {
    setBusyID(user.id);
    try {
      const result = await put<{ user: User }>(`/api/admin/rankings/${user.id}`, { hidden: !user.ranking_hidden });
      onChanged(result.user);
      toast.success(result.user.ranking_hidden ? `已屏蔽 ${user.display_name} 的排名` : `已恢复 ${user.display_name} 的排名`);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "排名设置保存失败");
    } finally {
      setBusyID(null);
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-6 overflow-auto bg-background p-4 sm:p-6 lg:p-8">
      <Card>
        <CardHeader>
          <CardTitle>排名展示账号</CardTitle>
          <CardDescription>屏蔽只会让账号从全部频道排行榜消失，不影响登录、频道成员身份和牌局数据。</CardDescription>
          <CardAction><Badge variant={hiddenCount > 0 ? "secondary" : "outline"}>{hiddenCount} 个已屏蔽</Badge></CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <InputGroup className="max-w-md"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder="搜索账号或显示名称" aria-label="搜索排名账号" /></InputGroup>
          {users.length === 0 ? (
            <Empty className="min-h-64 border"><EmptyHeader><EmptyMedia variant="icon"><Trophy /></EmptyMedia><EmptyTitle>没有匹配的账号</EmptyTitle><EmptyDescription>换一个用户名或显示名称试试。</EmptyDescription></EmptyHeader></Empty>
          ) : (
            <div className="overflow-hidden rounded-xl border">
              <Table>
                <TableHeader><TableRow><TableHead>账号</TableHead><TableHead>角色</TableHead><TableHead>排名状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>{users.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell><div className="flex items-center gap-3"><Avatar><AvatarFallback>{initials(user.display_name)}</AvatarFallback></Avatar><span className="min-w-0"><strong className="block truncate">{user.display_name}</strong><small className="block truncate text-muted-foreground">@{user.username}</small></span></div></TableCell>
                    <TableCell><Badge variant="outline">{user.role_name || user.role}</Badge></TableCell>
                    <TableCell><Badge variant={user.ranking_hidden ? "secondary" : "outline"}>{user.ranking_hidden ? "已屏蔽" : "正常展示"}</Badge></TableCell>
                    <TableCell className="text-right"><Button size="sm" variant={user.ranking_hidden ? "default" : "outline"} disabled={busyID !== null} onClick={() => void toggle(user)}>{busyID === user.id ? <Spinner data-icon="inline-start" /> : user.ranking_hidden ? <Eye data-icon="inline-start" /> : <EyeOff data-icon="inline-start" />}{user.ranking_hidden ? "恢复排名" : "屏蔽排名"}</Button></TableCell>
                  </TableRow>
                ))}</TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function AdminLoading() {
  return <div className="flex flex-col gap-6 p-4 sm:p-6 lg:p-8"><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-28" />)}</div><Skeleton className="h-36" /><Skeleton className="h-96" /></div>;
}

function withUser(overview: AdminOverview, nextUser: User): AdminOverview {
  const exists = overview.users.some((user) => user.id === nextUser.id);
  const users = exists ? overview.users.map((user) => user.id === nextUser.id ? nextUser : user) : [nextUser, ...overview.users];
  return recalculateOverview(overview, users);
}

function withoutUser(overview: AdminOverview, userID: number): AdminOverview {
  return recalculateOverview(overview, overview.users.filter((user) => user.id !== userID));
}

function recalculateOverview(overview: AdminOverview, users: User[]): AdminOverview {
  const counts: Record<string, number> = { total: users.length, active: users.filter((user) => user.status === "active").length };
  for (const user of users) counts[user.role] = (counts[user.role] || 0) + 1;
  const roles = overview.roles.map((role) => ({ ...role, user_count: users.filter((user) => user.role === role.key).length }));
  return { ...overview, users, counts, roles };
}

function roleLabel(role: UserRole, roles: Role[]) {
  return roles.find((item) => item.key === role)?.name || role;
}

function roleUsesChannelScope(roles: Role[], roleKey: string) {
  if (roleKey === "super_admin") return false;
  const permissions = roles.find((role) => role.key === roleKey)?.permissions || [];
  return permissions.includes("channels:manage") || permissions.includes("balances:manage");
}

function initials(name: string) {
  return name.trim().slice(0, 2).toUpperCase();
}
