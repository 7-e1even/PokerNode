import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from "react";
import { ImageUp, Save, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, patch, upload } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Spinner } from "@/components/ui/spinner";
import type { User } from "@/types";

const maxAvatarBytes = 2 * 1024 * 1024;
const acceptedAvatarTypes = new Set(["image/jpeg", "image/png", "image/webp", "image/gif"]);

export function ProfileSettings({ user, wechatLoginEnabled, onUpdated }: {
  user: User;
  wechatLoginEnabled: boolean;
  onUpdated: (user: User) => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [displayName, setDisplayName] = useState(user.display_name);
  const [avatarPreview, setAvatarPreview] = useState("");
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => setDisplayName(user.display_name), [user.display_name]);

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

  async function saveProfile(event: FormEvent) {
    event.preventDefault();
    setError("");
    const nextDisplayName = displayName.trim();
    if (!nextDisplayName) {
      setError("显示名称不能为空");
      return;
    }
    if (nextDisplayName === user.display_name) {
      setError("显示名称没有变化");
      return;
    }
    setSaving(true);
    try {
      const result = await patch<{ user: User }>("/api/me/profile", { display_name: nextDisplayName });
      onUpdated(result.user);
      toast.success("个人资料已保存");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "个人资料保存失败");
    } finally {
      setSaving(false);
    }
  }

  async function selectAvatar(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setError("");
    if (file.type && !acceptedAvatarTypes.has(file.type)) {
      setError("只支持 JPG、PNG、WebP 或 GIF 头像");
      return;
    }
    if (file.size > maxAvatarBytes) {
      setError("头像不能超过 2 MB");
      return;
    }
    const previewURL = URL.createObjectURL(file);
    setAvatarPreview(previewURL);
    setUploading(true);
    try {
      const body = new FormData();
      body.append("avatar", file);
      const result = await upload<{ user: User }>("/api/me/avatar", body);
      onUpdated(result.user);
      toast.success("头像已更新");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "头像上传失败");
    } finally {
      URL.revokeObjectURL(previewURL);
      setAvatarPreview("");
      setUploading(false);
    }
  }

  async function removeAvatar() {
    setRemoving(true);
    setError("");
    try {
      const result = await api<{ user: User }>("/api/me/avatar", { method: "DELETE" });
      onUpdated(result.user);
      toast.success("头像已移除");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "头像移除失败");
    } finally {
      setRemoving(false);
    }
  }

  return (
    <form onSubmit={saveProfile}>
      <Card>
        <CardHeader>
          <CardTitle>个人资料</CardTitle>
          <CardDescription>管理其他玩家看到的头像和显示名称。</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
          <FieldGroup>
            <Field orientation="responsive">
              <FieldContent>
                <FieldTitle>头像</FieldTitle>
                <FieldDescription>支持 JPG、PNG、WebP 和 GIF，文件不超过 2 MB。</FieldDescription>
              </FieldContent>
              <div className="flex items-center gap-3">
                <Avatar className="size-20">
                  <AvatarImage src={avatarPreview || user.avatar_url} alt={user.display_name} />
                  <AvatarFallback className="text-lg">{initials(user.display_name)}</AvatarFallback>
                </Avatar>
                <div className="flex flex-wrap gap-2">
                  <Input ref={fileInput} className="hidden" type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={(event) => void selectAvatar(event)} tabIndex={-1} />
                  <Button type="button" variant="outline" disabled={uploading || removing} onClick={() => fileInput.current?.click()}>
                    {uploading ? <Spinner data-icon="inline-start" /> : <ImageUp data-icon="inline-start" />}{uploading ? "上传中" : "上传头像"}
                  </Button>
                  {user.avatar_url && (
                    <Button type="button" variant="ghost" disabled={uploading || removing} onClick={() => void removeAvatar()}>
                      {removing ? <Spinner data-icon="inline-start" /> : <Trash2 data-icon="inline-start" />}{removing ? "移除中" : "移除"}
                    </Button>
                  )}
                </div>
              </div>
            </Field>
            <Separator />
            <Field>
              <FieldLabel htmlFor="profile-display-name">显示名称</FieldLabel>
              <Input id="profile-display-name" value={displayName} onChange={(event) => { setDisplayName(event.target.value); setError(""); }} maxLength={32} autoComplete="name" required />
              <FieldDescription>最多 32 个字符，会显示在排行榜、频道和牌桌中。</FieldDescription>
            </Field>
            {wechatLoginEnabled && (
              <>
                <Separator />
                <Field orientation="responsive">
                  <FieldContent>
                    <FieldTitle>微信账号</FieldTitle>
                    <FieldDescription>{user.wechat_bound ? "微信登录已绑定到当前 PokerNode 账号。" : "绑定后可以使用微信登录当前账号。"}</FieldDescription>
                  </FieldContent>
                  {user.wechat_bound ? <Badge variant="secondary">已绑定</Badge> : <Button type="button" variant="outline" onClick={() => window.location.assign("/api/auth/wechat/link")}>绑定微信</Button>}
                </Field>
              </>
            )}
          </FieldGroup>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={saving || uploading || removing}>{saving ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}{saving ? "保存中" : "保存资料"}</Button>
        </CardFooter>
      </Card>
    </form>
  );
}

function initials(name: string) {
  return name.trim().slice(0, 2).toUpperCase();
}
