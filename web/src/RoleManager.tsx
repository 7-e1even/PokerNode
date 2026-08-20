import { useMemo, useState, type FormEvent } from "react";
import { KeyRound, Pencil, Plus, ShieldCheck, Trash2, UsersRound } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogMedia, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldContent, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { patch, post, remove } from "./api";
import type { PermissionDefinition, Role } from "./types";

export default function RoleManager({ roles, catalog, grantablePermissions, onChanged }: {
  roles: Role[];
  catalog: PermissionDefinition[];
  grantablePermissions: string[];
  onChanged: (roles: Role[]) => void;
}) {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Role | null>(null);
  const [deleting, setDeleting] = useState<Role | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);

  async function deleteRole() {
    if (!deleting) return;
    setDeleteBusy(true);
    try {
      await remove(`/api/admin/roles/${encodeURIComponent(deleting.key)}`);
      onChanged(roles.filter((role) => role.key !== deleting.key));
      toast.success(`角色“${deleting.name}”已删除`);
      setDeleting(null);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "删除角色失败");
    } finally {
      setDeleteBusy(false);
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-6 overflow-auto bg-background p-4 sm:p-6 lg:p-8">
      <section className="grid gap-4 sm:grid-cols-3">
        <Metric icon={<ShieldCheck />} label="角色总数" value={roles.length} hint={`${roles.filter((role) => !role.system).length} 个自定义角色`} />
        <Metric icon={<UsersRound />} label="已分配用户" value={roles.reduce((total, role) => total + role.user_count, 0)} hint="按主角色统计" />
        <Metric icon={<KeyRound />} label="功能权限" value={catalog.length} hint="频道范围另行分配" />
      </section>

      <Card>
        <CardHeader>
          <CardTitle>角色与功能权限</CardTitle>
          <CardDescription>仅超级管理员为系统内置角色；其余默认角色可自由修改，频道管理范围在用户账号中分配。</CardDescription>
          <CardAction><Button onClick={() => setCreating(true)}><Plus data-icon="inline-start" />创建角色</Button></CardAction>
        </CardHeader>
        <CardContent>
          {roles.length === 0 ? (
            <Empty className="min-h-56"><EmptyHeader><EmptyMedia variant="icon"><ShieldCheck /></EmptyMedia><EmptyTitle>还没有角色</EmptyTitle><EmptyDescription>创建角色后即可为用户分配权限。</EmptyDescription></EmptyHeader></Empty>
          ) : (
            <>
              <div className="hidden overflow-x-auto sm:block">
                <Table>
                  <TableHeader><TableRow><TableHead>角色</TableHead><TableHead>功能权限</TableHead><TableHead>用户</TableHead><TableHead>类型</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                  <TableBody>{roles.map((role) => (
                    <TableRow key={role.key}>
                      <TableCell><strong className="block">{role.name}</strong><small className="text-muted-foreground">{role.key}</small>{role.description && <p className="mt-1 max-w-72 text-xs text-muted-foreground">{role.description}</p>}</TableCell>
                      <TableCell><div className="flex max-w-xl flex-wrap gap-1.5">{role.permissions.length ? role.permissions.map((key) => <Badge key={key} variant="outline">{catalog.find((item) => item.key === key)?.name || key}</Badge>) : <span className="text-muted-foreground">无后台功能权限</span>}</div></TableCell>
                      <TableCell className="tabular-nums">{role.user_count}</TableCell>
                      <TableCell><Badge variant={role.system ? "secondary" : "outline"}>{role.system ? "系统内置" : "自定义"}</Badge></TableCell>
                      <TableCell className="text-right"><span className="inline-flex gap-2"><Button size="sm" variant="outline" disabled={role.system} onClick={() => setEditing(role)}><Pencil data-icon="inline-start" />编辑</Button><Button size="icon-sm" variant="ghost" disabled={role.system} aria-label={`删除角色 ${role.name}`} onClick={() => setDeleting(role)}><Trash2 /></Button></span></TableCell>
                    </TableRow>
                  ))}</TableBody>
                </Table>
              </div>
              <div className="flex flex-col gap-3 sm:hidden">
                {roles.map((role) => (
                  <Card key={role.key}>
                    <CardHeader>
                      <CardTitle>{role.name}</CardTitle>
                      <CardDescription>{role.key}{role.description ? ` · ${role.description}` : ""}</CardDescription>
                      <CardAction><Badge variant={role.system ? "secondary" : "outline"}>{role.system ? "系统内置" : `${role.user_count} 名用户`}</Badge></CardAction>
                    </CardHeader>
                    <CardContent className="flex flex-col gap-3">
                      <div className="flex flex-wrap gap-1.5">{role.permissions.length ? role.permissions.map((key) => <Badge key={key} variant="outline">{catalog.find((item) => item.key === key)?.name || key}</Badge>) : <span className="text-sm text-muted-foreground">无后台功能权限</span>}</div>
                      <div className="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={role.system} onClick={() => setEditing(role)}><Pencil data-icon="inline-start" />编辑</Button><Button size="icon-sm" variant="ghost" disabled={role.system} aria-label={`删除角色 ${role.name}`} onClick={() => setDeleting(role)}><Trash2 /></Button></div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {creating && <RoleDialog catalog={catalog} grantablePermissions={grantablePermissions} onClose={() => setCreating(false)} onSaved={(role) => { onChanged([...roles, role]); setCreating(false); toast.success("角色已创建"); }} />}
      {editing && <RoleDialog role={editing} catalog={catalog} grantablePermissions={grantablePermissions} onClose={() => setEditing(null)} onSaved={(role) => { onChanged(roles.map((item) => item.key === role.key ? role : item)); setEditing(null); toast.success("角色权限已更新"); }} />}

      <AlertDialog open={deleting !== null} onOpenChange={(open) => !open && !deleteBusy && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogMedia><Trash2 /></AlertDialogMedia><AlertDialogTitle>删除角色“{deleting?.name}”？</AlertDialogTitle><AlertDialogDescription>{deleting?.user_count ? `当前仍有 ${deleting.user_count} 名用户使用该角色，请先调整这些用户的角色。` : "删除后无法恢复；系统内置角色不会受影响。"}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel disabled={deleteBusy}>取消</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={deleteBusy || (deleting?.user_count || 0) > 0} onClick={(event) => { event.preventDefault(); void deleteRole(); }}>{deleteBusy && <Spinner data-icon="inline-start" />}{deleteBusy ? "正在删除…" : "确认删除"}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function RoleDialog({ role, catalog, grantablePermissions, onClose, onSaved }: { role?: Role; catalog: PermissionDefinition[]; grantablePermissions: string[]; onClose: () => void; onSaved: (role: Role) => void }) {
  const [key, setKey] = useState(role?.key || "");
  const [name, setName] = useState(role?.name || "");
  const [description, setDescription] = useState(role?.description || "");
  const [permissions, setPermissions] = useState<string[]>(() => expandPermissionSelection(role?.permissions || [], catalog));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const groups = useMemo(() => Array.from(new Set(catalog.map((permission) => permission.group))), [catalog]);
  const grantable = useMemo(() => new Set(grantablePermissions), [grantablePermissions]);
  const ungrantable = permissions.filter((permission) => !grantable.has(permission));

  function toggle(permission: string, checked: boolean) {
    setPermissions((current) => checked
      ? expandPermissionSelection([...current, permission], catalog)
      : removePermissionAndDependents(current, permission, catalog));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const body = { key, name, description, permissions };
      const result = role
        ? await patch<{ role: Role }>(`/api/admin/roles/${encodeURIComponent(role.key)}`, body)
        : await post<{ role: Role }>("/api/admin/roles", body);
      onSaved(result.role);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "保存角色失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[min(90vh,760px)] overflow-y-auto sm:max-w-xl">
        <form onSubmit={submit}>
          <DialogHeader><DialogTitle>{role ? `编辑角色“${role.name}”` : "创建自定义角色"}</DialogTitle><DialogDescription>功能权限控制操作能力；拥有“管理频道”的用户还需在账号中分配一个或多个频道。</DialogDescription></DialogHeader>
          <FieldGroup className="mt-6">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field><FieldLabel htmlFor="role-name">角色名称</FieldLabel><Input id="role-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：频道运营" required autoFocus /></Field>
              <Field data-disabled={!!role}><FieldLabel htmlFor="role-key">角色标识</FieldLabel><Input id="role-key" value={key} disabled={!!role} onChange={(event) => setKey(event.target.value.toLowerCase())} placeholder="channel_operator" required /><FieldDescription>小写字母、数字或下划线，创建后不可修改。</FieldDescription></Field>
            </div>
            <Field><FieldLabel htmlFor="role-description">角色说明</FieldLabel><Textarea id="role-description" value={description} onChange={(event) => setDescription(event.target.value)} placeholder="说明这个角色负责什么" maxLength={200} /></Field>
            <Field><FieldLabel>功能权限</FieldLabel><FieldDescription>按职责最小化授权；依赖权限会自动选中，且只能授予当前账号已拥有的权限。</FieldDescription></Field>
            <div className="grid gap-4 sm:grid-cols-2">
              {groups.map((group) => <div key={group} className="rounded-xl border p-3"><strong className="mb-3 block text-sm">{group}</strong><FieldGroup className="gap-3">{catalog.filter((item) => item.group === group).map((item) => {
                const canGrant = grantable.has(item.key);
                const requiredNames = (item.requires || []).map((key) => catalog.find((candidate) => candidate.key === key)?.name || key);
                return <Field key={item.key} orientation="horizontal" data-disabled={!canGrant}><FieldContent><FieldTitle>{item.name}{!canGrant && <Badge className="ml-2" variant="outline">不可授予</Badge>}</FieldTitle><FieldDescription>{item.description}{requiredNames.length ? ` · 依赖：${requiredNames.join("、")}` : ""}</FieldDescription></FieldContent><Switch checked={permissions.includes(item.key)} disabled={!canGrant} onCheckedChange={(checked) => toggle(item.key, checked)} aria-label={item.name} /></Field>;
              })}</FieldGroup></div>)}
            </div>
            {ungrantable.length > 0 && <Alert><AlertDescription>该角色包含当前账号无权授予的权限，因此不能由当前账号保存。</AlertDescription></Alert>}
            <FieldError>{error}</FieldError>
          </FieldGroup>
          <DialogFooter className="mt-6"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button disabled={busy || ungrantable.length > 0}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在保存…" : "保存角色"}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function expandPermissionSelection(selected: string[], catalog: PermissionDefinition[]) {
  const result = new Set(selected);
  for (let changed = true; changed;) {
    changed = false;
    for (const item of catalog) {
      if (!result.has(item.key)) continue;
      for (const required of item.requires || []) {
        if (result.has(required)) continue;
        result.add(required);
        changed = true;
      }
    }
  }
  return catalog.filter((item) => result.has(item.key)).map((item) => item.key);
}

function removePermissionAndDependents(selected: string[], removed: string, catalog: PermissionDefinition[]) {
  const result = new Set(selected);
  result.delete(removed);
  for (let changed = true; changed;) {
    changed = false;
    for (const item of catalog) {
      if (!result.has(item.key) || (item.requires || []).every((required) => result.has(required))) continue;
      result.delete(item.key);
      changed = true;
    }
  }
  return catalog.filter((item) => result.has(item.key)).map((item) => item.key);
}

function Metric({ icon, label, value, hint }: { icon: React.ReactNode; label: string; value: number; hint: string }) {
  return <Card><CardContent className="flex items-center gap-4 p-5"><span className="grid size-10 place-items-center rounded-lg bg-muted text-muted-foreground">{icon}</span><span><small className="block text-muted-foreground">{label}</small><strong className="font-heading text-2xl">{value}</strong><span className="ml-2 text-xs text-muted-foreground">{hint}</span></span></CardContent></Card>;
}
