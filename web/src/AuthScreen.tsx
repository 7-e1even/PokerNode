import { LoginForm } from "@/components/login-form"
import type { User } from "@/types"

export default function AuthScreen({
  registrationEnabled,
  wechatLoginEnabled,
  onAuthenticated,
}: {
  registrationEnabled: boolean
  wechatLoginEnabled: boolean
  onAuthenticated: (user: User) => void
}) {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center bg-muted p-6 md:p-10">
      <div className="w-full max-w-sm md:max-w-5xl">
        <LoginForm
          registrationEnabled={registrationEnabled}
          wechatLoginEnabled={wechatLoginEnabled}
          onAuthenticated={onAuthenticated}
        />
      </div>
    </main>
  )
}
