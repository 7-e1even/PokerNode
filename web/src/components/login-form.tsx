import { useEffect, useState, type ComponentProps, type FormEvent, type SVGProps } from "react"
import {
  ArrowRightIcon,
  Link2Icon,
} from "lucide-react"
import { post } from "@/api"
import { LoginHeroImage } from "@/components/login-hero-image"
import { WeChatIcon } from "@/components/wechat-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import type { LoginHeroConfig, User } from "@/types"

interface LoginFormProps extends Omit<ComponentProps<"div">, "onSubmit"> {
  siteName: string
  registrationEnabled: boolean
  wechatLoginEnabled: boolean
  loginHero: LoginHeroConfig
  onAuthenticated: (user: User) => void
}

interface LoginProvider {
  id: string
  label: string
  href: string
  Icon: (props: SVGProps<SVGSVGElement>) => React.JSX.Element
}

export function LoginForm({
  className,
  siteName,
  registrationEnabled,
  wechatLoginEnabled,
  loginHero,
  onAuthenticated,
  ...props
}: LoginFormProps) {
  const [mode, setMode] = useState<"login" | "register">("login")
  const [username, setUsername] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  const loginProviders: LoginProvider[] = [
    ...(wechatLoginEnabled
      ? [{ id: "wechat", label: "微信登录", href: "/api/auth/wechat/start", Icon: WeChatIcon }]
      : []),
  ]

  useEffect(() => {
    if (!registrationEnabled) setMode("login")
  }, [registrationEnabled])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const result = params.get("wechat_error")
    if (!result) return

    const messages: Record<string, string> = {
      registration_closed: "当前已关闭新用户注册。老用户可先用账号密码登录，再从头像菜单绑定微信。",
      disabled: "这个微信关联的账号已停用，请联系管理员。",
      cancelled: "已取消微信授权。",
      invalid_state: "微信授权已失效，请重新发起登录。",
      provider_failed: "微信授权失败，请稍后重试。",
      unavailable: "微信登录尚未配置。",
      failed: "微信登录失败，请稍后重试。",
    }
    setError(messages[result] ?? messages.failed)
    params.delete("wechat_error")
    const query = params.toString()
    window.history.replaceState(
      {},
      "",
      `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`,
    )
  }, [])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError("")
    try {
      const result = await post<{ user: User }>(`/api/auth/${mode}`, {
        username,
        password,
        display_name: displayName,
      })
      onAuthenticated(result.user)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "请求失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card className="overflow-hidden p-0">
        <CardContent className="grid p-0 md:grid-cols-2">
          <form className="flex items-center p-6 md:p-10" onSubmit={submit}>
            <FieldGroup>
              <div className="flex flex-col items-center gap-2 text-center">
                <h1 className="text-2xl font-bold">
                  {mode === "login" ? "欢迎回来" : "创建牌手账号"}
                </h1>
                <p className="text-balance text-muted-foreground">
                  {mode === "login" ? `登录你的 ${siteName} 账号` : "创建账号后将自动登录"}
                </p>
              </div>

              <Field>
                <FieldLabel htmlFor="username">用户名</FieldLabel>
                <Input
                  id="username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  autoComplete="username"
                  placeholder="3–24 位字母或数字"
                  required
                />
              </Field>

              {mode === "register" && (
                <Field>
                  <FieldLabel htmlFor="display-name">显示名称</FieldLabel>
                  <Input
                    id="display-name"
                    value={displayName}
                    onChange={(event) => setDisplayName(event.target.value)}
                    autoComplete="nickname"
                    placeholder="牌桌上显示的名字"
                  />
                </Field>
              )}

              <Field>
                <FieldLabel htmlFor="password">密码</FieldLabel>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete={mode === "login" ? "current-password" : "new-password"}
                  placeholder="至少 8 位"
                  required
                />
              </Field>

              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}

              <Field>
                <Button size="lg" disabled={busy}>
                  {busy && <Spinner data-icon="inline-start" />}
                  {busy ? "请稍候…" : mode === "login" ? "登录" : "创建并登录"}
                  {!busy && <ArrowRightIcon data-icon="inline-end" />}
                </Button>
              </Field>

              {loginProviders.length > 0 && (
                <>
                  <FieldSeparator className="*:data-[slot=field-separator-content]:bg-card">
                    其他登录方式
                  </FieldSeparator>
                  <Field className={cn(
                    "grid gap-4",
                    loginProviders.length === 1 && "grid-cols-1",
                    loginProviders.length === 2 && "grid-cols-2",
                    loginProviders.length >= 3 && "grid-cols-3",
                  )}>
                    {loginProviders.map(({ id, label, href, Icon }) => (
                      <Button
                        key={id}
                        variant="outline"
                        size="lg"
                        type="button"
                        onClick={() => window.location.assign(href)}
                      >
                        <Icon data-icon="inline-start" />
                        <span className={cn(loginProviders.length > 1 && "sr-only")}>{label}</span>
                      </Button>
                    ))}
                  </Field>
                  {loginProviders.some(({ id }) => id === "wechat") && (
                    <FieldDescription className="flex items-start justify-center gap-2 text-center">
                      <Link2Icon className="mt-0.5 size-3.5 shrink-0" />
                      微信首次登录会自动创建账号；老用户请登录后从头像菜单绑定微信。
                    </FieldDescription>
                  )}
                </>
              )}

              <FieldDescription className="text-center">
                {mode === "login" ? (
                  registrationEnabled ? (
                    <>还没有账号？ <ModeButton onClick={() => setMode("register")}>创建账号</ModeButton></>
                  ) : (
                    "平台已关闭自助注册，新账号请联系管理员创建。"
                  )
                ) : (
                  <>已有账号？ <ModeButton onClick={() => setMode("login")}>返回登录</ModeButton></>
                )}
              </FieldDescription>
            </FieldGroup>
          </form>

          <section className="relative hidden min-h-[640px] overflow-hidden bg-muted md:block">
            <LoginHeroImage config={loginHero} className="absolute inset-0" />
          </section>
        </CardContent>
      </Card>
    </div>
  )
}

function ModeButton({ children, onClick }: { children: string; onClick: () => void }) {
  return (
    <button type="button" className="font-medium underline underline-offset-4" onClick={onClick}>
      {children}
    </button>
  )
}
