import { useState, type FormEvent } from "react";
import { ArrowLeft, ArrowRight, LockKeyhole, ShieldCheck } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { BrandMark } from "@/components/brand-mark";
import { post } from "./api";
import type { User } from "./types";

export default function AdminAuthScreen({ currentUser, onAuthenticated, onBack }: {
  currentUser?: User | null;
  onAuthenticated: (user: User) => void;
  onBack: () => void;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await post<{ user: User }>("/api/admin/auth/login", { username, password });
      onAuthenticated(result.user);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "登录运营后台失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="admin-auth grid min-h-svh lg:grid-cols-[minmax(0,1fr)_30rem]">
      <section className="admin-auth__story relative hidden overflow-hidden border-r p-14 lg:flex lg:flex-col lg:justify-between">
        <div className="relative flex items-center gap-3 font-heading text-lg font-semibold">
          <BrandMark className="size-10" />
          PokerNode Operations
        </div>
        <div className="relative max-w-2xl">
          <Badge className="mb-6" variant="outline"><ShieldCheck data-icon="inline-start" />受控运营入口</Badge>
          <h1 className="font-heading text-5xl leading-tight font-semibold tracking-tight">账号、频道和运行状态，<br />集中管理。</h1>
          <p className="mt-6 max-w-xl text-lg leading-8 text-muted-foreground">运营后台与玩家大厅使用独立入口，只展示当前角色获准查看和操作的功能。</p>
        </div>
        <div className="relative grid max-w-2xl grid-cols-3 gap-3 text-sm">
          <AuthFeature value="RBAC" label="角色权限" />
          <AuthFeature value="New API" label="频道节点" />
          <AuthFeature value="Audit" label="资金流水" />
        </div>
      </section>

      <section className="flex items-center justify-center bg-background p-5 text-foreground sm:p-10">
        <div className="w-full max-w-sm">
          <Button className="mb-8 -ml-3" variant="ghost" onClick={onBack}><ArrowLeft data-icon="inline-start" />返回玩家大厅</Button>
          <Card>
            <CardHeader>
              <span className="mb-3 grid size-11 place-items-center rounded-xl bg-primary text-primary-foreground"><LockKeyhole /></span>
              <CardTitle className="text-2xl">运营后台登录</CardTitle>
              <CardDescription>仅拥有后台访问权限的账号可以进入</CardDescription>
            </CardHeader>
            <CardContent>
              {currentUser && !currentUser.permissions?.includes("admin:view") && (
                <Alert className="mb-5"><AlertDescription>当前账号 @{currentUser.username} 没有后台访问权限，请使用有权限的账号切换登录。</AlertDescription></Alert>
              )}
              <form onSubmit={submit}>
                <FieldGroup>
                  <Field><FieldLabel htmlFor="admin-username">管理员账号</FieldLabel><Input id="admin-username" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" autoFocus required /></Field>
                  <Field><FieldLabel htmlFor="admin-password">密码</FieldLabel><Input id="admin-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></Field>
                  {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
                  <Button size="lg" disabled={busy}>{busy && <Spinner data-icon="inline-start" />}{busy ? "正在验证权限…" : "进入运营后台"}{!busy && <ArrowRight data-icon="inline-end" />}</Button>
                </FieldGroup>
              </form>
            </CardContent>
          </Card>
          <p className="mt-5 text-center text-xs text-muted-foreground">后台会话使用 HttpOnly Cookie；无后台访问权限的账号会被拒绝。</p>
        </div>
      </section>
    </main>
  );
}

function AuthFeature({ value, label }: { value: string; label: string }) {
  return <div className="rounded-xl border bg-card p-4"><strong className="block">{value}</strong><span className="text-muted-foreground">{label}</span></div>;
}
