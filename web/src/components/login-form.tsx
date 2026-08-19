import { useEffect, useState, type ComponentProps, type FormEvent, type SVGProps } from "react"
import {
  ArrowRightIcon,
  CircleDollarSignIcon,
  Layers3Icon,
  Link2Icon,
  ShieldCheckIcon,
  SparklesIcon,
} from "lucide-react"
import { post } from "@/api"
import { BrandMark } from "@/components/brand-mark"
import { WeChatIcon } from "@/components/wechat-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
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
import type { User } from "@/types"

interface LoginFormProps extends Omit<ComponentProps<"div">, "onSubmit"> {
  registrationEnabled: boolean
  wechatLoginEnabled: boolean
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
  registrationEnabled,
  wechatLoginEnabled,
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
                <BrandMark className="mb-2 size-11" />
                <h1 className="text-2xl font-bold">
                  {mode === "login" ? "欢迎回来" : "创建牌手账号"}
                </h1>
                <p className="text-balance text-muted-foreground">
                  {mode === "login" ? "登录 PokerNode，继续朋友间的牌局" : "创建账号后将自动登录"}
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

          <PokerNodeStory />
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

function PokerNodeStory() {
  return (
    <section className="relative hidden min-h-[640px] overflow-hidden bg-muted p-8 md:flex md:flex-col md:justify-between">
      <div className="relative flex items-center gap-3 font-heading text-lg font-semibold">
        <BrandMark className="size-10" />
        PokerNode
      </div>

      <div className="relative">
        <Badge variant="outline" className="mb-5 rounded-full bg-background/70 px-3 py-1 backdrop-blur">
          <SparklesIcon data-icon="inline-start" /> 私有牌局频道
        </Badge>
        <h2 className="font-heading text-4xl leading-[1.08] font-semibold tracking-tight">
          朋友间的牌局，
          <br />一处就够了。
        </h2>
        <p className="mt-5 max-w-sm leading-7 text-muted-foreground">
          频道、牌桌与实时结算由 PokerNode 统一管理，每个频道的数据和凭证彼此隔离。
        </p>
        <div className="mt-6 flex flex-wrap gap-2">
          <Badge variant="secondary"><Layers3Icon data-icon="inline-start" />频道隔离</Badge>
          <Badge variant="secondary"><ShieldCheckIcon data-icon="inline-start" />凭证加密</Badge>
          <Badge variant="secondary"><CircleDollarSignIcon data-icon="inline-start" />实时结算</Badge>
        </div>
      </div>

      <div className="relative rounded-xl border bg-card/80 p-4 shadow-sm backdrop-blur">
        <div className="mb-4 flex items-center justify-between">
          <div>
            <p className="text-xs text-muted-foreground">当前底池</p>
            <p className="mt-1 text-xl font-semibold">$128.40</p>
          </div>
          <Badge>River</Badge>
        </div>
        <div className="flex gap-2">
          <PreviewCard value="A♦" red />
          <PreviewCard value="K♠" />
          <PreviewCard value="10♥" red />
          <PreviewCard value="7♣" />
          <PreviewCard value="2♠" />
        </div>
      </div>
    </section>
  )
}

function PreviewCard({ value, red = false }: { value: string; red?: boolean }) {
  return (
    <div className={cn(
      "grid aspect-[3/4] flex-1 place-items-center rounded-lg border bg-card font-heading text-sm font-semibold shadow-sm",
      red && "text-destructive",
    )}>
      {value}
    </div>
  )
}
