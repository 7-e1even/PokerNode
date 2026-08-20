import { LoginForm } from "@/components/login-form"
import type { LoginHeroConfig, User } from "@/types"

export default function AuthScreen({
  registrationEnabled,
  wechatLoginEnabled,
  loginHero,
  onAuthenticated,
}: {
  registrationEnabled: boolean
  wechatLoginEnabled: boolean
  loginHero: LoginHeroConfig
  onAuthenticated: (user: User) => void
}) {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center bg-muted p-6 md:p-10">
      <div className="w-full max-w-sm md:max-w-5xl">
        <LoginForm
          registrationEnabled={registrationEnabled}
          wechatLoginEnabled={wechatLoginEnabled}
          loginHero={loginHero}
          onAuthenticated={onAuthenticated}
        />
      </div>
    </main>
  )
}
