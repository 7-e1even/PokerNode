import { ArrowLeft, Bot, KeyRound, UserRound } from "lucide-react";
import { AccountSecuritySettings } from "@/components/account-security-settings";
import { BrandMark } from "@/components/brand-mark";
import { MCPKeySettings } from "@/components/mcp-key-settings";
import { ProfileSettings } from "@/components/profile-settings";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { User } from "@/types";

export default function SettingsPage({ user, wechatLoginEnabled, onBack, onUpdated }: {
  user: User;
  wechatLoginEnabled: boolean;
  onBack: () => void;
  onUpdated: (user: User) => void;
}) {
  return (
    <main className="min-h-svh bg-muted/30">
      <header className="border-b bg-background">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6">
          <Button type="button" size="icon" variant="ghost" onClick={onBack} aria-label="返回"><ArrowLeft /></Button>
          <BrandMark className="size-8" aria-hidden="true" />
          <div className="min-w-0">
            <strong className="block truncate font-heading">个人设置</strong>
            <span className="block truncate text-xs text-muted-foreground">{user.display_name} · @{user.username}</span>
          </div>
        </div>
      </header>
      <section className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-8 sm:px-6" aria-labelledby="settings-title">
        <div>
          <h1 id="settings-title" className="font-heading text-2xl font-semibold tracking-tight">个人设置</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理个人资料、登录安全和 Agent 连接。</p>
        </div>
        <Tabs defaultValue="profile" orientation="vertical" className="grid gap-6 md:grid-cols-[12rem_minmax(0,1fr)] md:items-start">
          <TabsList variant="line" className="grid w-full grid-cols-3 gap-1 md:flex md:w-48 md:shrink-0">
            <TabsTrigger className="min-h-10 px-3" value="profile"><UserRound />个人资料</TabsTrigger>
            <TabsTrigger className="min-h-10 px-3" value="security"><KeyRound />账号安全</TabsTrigger>
            <TabsTrigger className="min-h-10 px-3" value="mcp"><Bot />Agent MCP</TabsTrigger>
          </TabsList>
          <TabsContent className="min-w-0" value="profile"><ProfileSettings user={user} wechatLoginEnabled={wechatLoginEnabled} onUpdated={onUpdated} /></TabsContent>
          <TabsContent className="min-w-0" value="security"><AccountSecuritySettings user={user} onUpdated={onUpdated} /></TabsContent>
          <TabsContent className="min-w-0" value="mcp"><MCPKeySettings /></TabsContent>
        </Tabs>
      </section>
    </main>
  );
}
