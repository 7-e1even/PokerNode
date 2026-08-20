import { useEffect, useState, type FormEvent } from "react"
import { Save } from "lucide-react"
import { toast } from "sonner"

import { put } from "@/api"
import { WeChatIcon } from "@/components/wechat-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import type { WeChatLoginConfig } from "@/types"

export function WeChatLoginSettings({ settings, canManage, onChanged }: {
  settings: WeChatLoginConfig
  canManage: boolean
  onChanged: (settings: WeChatLoginConfig) => void
}) {
  const [appID, setAppID] = useState(settings.app_id)
  const [appSecret, setAppSecret] = useState("")
  const [redirectURI, setRedirectURI] = useState(settings.redirect_uri)
  const [enabled, setEnabled] = useState(settings.enabled)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    setAppID(settings.app_id)
    setAppSecret("")
    setRedirectURI(settings.redirect_uri)
    setEnabled(settings.enabled)
  }, [settings])

  async function save(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError("")
    try {
      const result = await put<{ wechat_login: WeChatLoginConfig }>("/api/admin/settings/wechat", {
        app_id: appID,
        app_secret: appSecret,
        redirect_uri: redirectURI,
        enabled,
      })
      setAppSecret("")
      onChanged(result.wechat_login)
      toast.success(result.wechat_login.enabled ? "微信登录已启用" : "微信登录配置已保存")
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : "微信登录配置保存失败"
      setError(message)
      toast.error(message)
    } finally {
      setBusy(false)
    }
  }

  const status = settings.enabled ? "已启用" : settings.configured ? "已停用" : "未配置"

  return (
    <Card>
      <form className="flex flex-col gap-(--card-spacing)" onSubmit={save}>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><WeChatIcon className="size-5" />微信登录</CardTitle>
          <CardDescription>配置微信开放平台网站应用。AppSecret 加密保存且不会返回前端。</CardDescription>
          <CardAction><Badge variant={settings.enabled ? "secondary" : "outline"}>{status}</Badge></CardAction>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            {settings.source === "environment" && (
              <Alert><AlertDescription>当前使用环境变量配置。首次保存到后台时需要重新输入 AppSecret，保存后数据库配置将优先生效。</AlertDescription></Alert>
            )}
            <Field data-disabled={!canManage}>
              <FieldLabel htmlFor="wechat-app-id">AppID</FieldLabel>
              <Input id="wechat-app-id" value={appID} onChange={(event) => setAppID(event.target.value)} disabled={!canManage || busy} autoComplete="off" placeholder="微信开放平台网站应用 AppID" />
            </Field>
            <Field data-disabled={!canManage}>
              <FieldLabel htmlFor="wechat-app-secret">AppSecret</FieldLabel>
              <Input id="wechat-app-secret" type="password" value={appSecret} onChange={(event) => setAppSecret(event.target.value)} disabled={!canManage || busy} autoComplete="new-password" placeholder={settings.source === "database" && settings.app_secret_configured ? "已配置，留空保持不变" : "请输入 AppSecret"} />
              <FieldDescription>密钥只在保存时提交；之后仅显示是否已经配置。</FieldDescription>
            </Field>
            <Field data-disabled={!canManage}>
              <FieldLabel htmlFor="wechat-redirect-uri">授权回调地址</FieldLabel>
              <Input id="wechat-redirect-uri" type="url" value={redirectURI} onChange={(event) => setRedirectURI(event.target.value)} disabled={!canManage || busy} autoComplete="url" placeholder="https://你的域名/api/auth/wechat/callback" />
              <FieldDescription>须与微信开放平台中登记的回调地址一致，路径固定为 /api/auth/wechat/callback。</FieldDescription>
            </Field>
            <Field orientation="horizontal" data-disabled={!canManage}>
              <FieldContent><FieldTitle>启用微信登录</FieldTitle><FieldDescription>关闭后登录页和个人绑定入口会立即隐藏，账号密码登录不受影响。</FieldDescription></FieldContent>
              <Switch checked={enabled} onCheckedChange={setEnabled} disabled={!canManage || busy} aria-label="启用微信登录" />
            </Field>
            <FieldError>{error}</FieldError>
            {!canManage && <Alert><AlertDescription>只有超级管理员可以修改微信登录配置。</AlertDescription></Alert>}
          </FieldGroup>
        </CardContent>
        <CardFooter className="justify-between gap-4">
          <span className="text-xs text-muted-foreground">配置来源：{settings.source === "database" ? "运营后台" : settings.source === "environment" ? "环境变量" : "未配置"}</span>
          <Button disabled={!canManage || busy}>{busy ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}{busy ? "正在保存…" : "保存微信配置"}</Button>
        </CardFooter>
      </form>
    </Card>
  )
}
