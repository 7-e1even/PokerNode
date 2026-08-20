import { useEffect, useState, type FormEvent } from "react";
import { Save } from "lucide-react";
import { toast } from "sonner";
import { patch } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import type { User } from "@/types";

export function AccountSecuritySettings({ user, onUpdated }: { user: User; onUpdated: (user: User) => void }) {
  const [username, setUsername] = useState(user.username);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [confirmError, setConfirmError] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const hasPassword = user.has_password !== false;

  useEffect(() => setUsername(user.username), [user.username]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setConfirmError("");
    if (newPassword !== confirmPassword) {
      setConfirmError("两次输入的新密码不一致");
      return;
    }
    if (!hasPassword && !newPassword) {
      setError("首次设置登录账号时必须同时设置密码");
      return;
    }
    if (username.trim() === user.username && !newPassword) {
      setError("账号或密码没有变化");
      return;
    }
    setSaving(true);
    try {
      const result = await patch<{ user: User }>("/api/me/credentials", {
        username,
        current_password: currentPassword,
        new_password: newPassword,
      });
      onUpdated(result.user);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      toast.success("账号安全设置已保存");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "账号更新失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={submit}>
      <Card>
        <CardHeader>
          <CardTitle>账号安全</CardTitle>
          <CardDescription>修改登录账号，或设置一个新的登录密码。</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-5">
            {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="account-username">登录账号</FieldLabel>
                <Input id="account-username" value={username} onChange={(event) => { setUsername(event.target.value); setError(""); }} autoComplete="username" minLength={3} maxLength={24} pattern="[a-zA-Z0-9_]+" required />
                <FieldDescription>3–24 位字母、数字或下划线。</FieldDescription>
              </Field>
              {hasPassword && (
                <Field>
                  <FieldLabel htmlFor="account-current-password">当前密码</FieldLabel>
                  <Input id="account-current-password" type="password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} autoComplete="current-password" required />
                  <FieldDescription>修改账号或密码前需要验证身份。</FieldDescription>
                </Field>
              )}
              <Field>
                <FieldLabel htmlFor="account-new-password">{hasPassword ? "新密码" : "设置密码"}</FieldLabel>
                <Input id="account-new-password" type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} autoComplete="new-password" minLength={8} maxLength={72} required={!hasPassword} />
                <FieldDescription>{hasPassword ? "不修改密码时可以留空；新密码需为 8–72 位。" : "设置后也可以使用账号密码登录。"}</FieldDescription>
              </Field>
              <Field data-invalid={!!confirmError || undefined}>
                <FieldLabel htmlFor="account-confirm-password">确认新密码</FieldLabel>
                <Input id="account-confirm-password" type="password" value={confirmPassword} onChange={(event) => { setConfirmPassword(event.target.value); setConfirmError(""); }} autoComplete="new-password" minLength={8} maxLength={72} required={!!newPassword} aria-invalid={!!confirmError || undefined} />
                {confirmError && <FieldError>{confirmError}</FieldError>}
              </Field>
            </FieldGroup>
          </div>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={saving}>{saving ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}{saving ? "保存中" : "保存安全设置"}</Button>
        </CardFooter>
      </Card>
    </form>
  );
}
