import { useMemo } from "react";
import { AppSidebar, type AdminSection } from "@/components/app-sidebar";
import { NavUser } from "@/components/nav-user";
import { SiteHeader } from "@/components/site-header";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { post } from "./api";
import AdminConsole from "./AdminConsole";
import type { LoginHeroConfig, User } from "./types";

const sectionCopy: Record<AdminSection, { title: string; description: string }> = {
  overview: { title: "平台总览", description: "账号、频道、牌桌和资金流水的实时概况" },
  users: { title: "用户管理", description: "创建、停用账号，分配角色与可管理频道" },
  roles: { title: "角色管理", description: "自定义角色并配置每个角色的功能权限" },
  channels: { title: "频道管理", description: "查看有权管理的频道、成员绑定、牌桌和结算状态" },
  balances: { title: "余额管理", description: "查看并调整全部频道成员的 New API 余额" },
  rankings: { title: "排名管理", description: "控制哪些账号会出现在频道排行榜中" },
  settings: { title: "系统设置", description: "控制登录方式、注册策略与登录页展示" },
};

export default function AdminDashboard({ user, section, onSectionChanged, onOpenLobby, onOpenSettings, onLogout, onRegistrationChanged, onLoginHeroChanged, onWeChatLoginChanged }: {
  user: User;
  section: AdminSection;
  onSectionChanged: (section: AdminSection) => void;
  onOpenLobby: () => void;
  onOpenSettings: () => void;
  onLogout: () => void;
  onRegistrationChanged: (enabled: boolean) => void;
  onLoginHeroChanged: (config: LoginHeroConfig) => void;
  onWeChatLoginChanged: (enabled: boolean) => void;
}) {
  const copy = useMemo(() => sectionCopy[section], [section]);

  async function logout() {
    await post("/api/auth/logout");
    onLogout();
  }

  return (
    <SidebarProvider defaultOpen>
      <AppSidebar user={user} active={section} onNavigate={onSectionChanged} />
      <SidebarInset className="min-h-svh overflow-hidden">
        <SiteHeader title={copy.title} description={copy.description} actions={<NavUser user={user} onOpenLobby={onOpenLobby} onOpenSettings={onOpenSettings} onLogout={() => void logout()} />} />
        <AdminConsole currentUser={user} section={section} onRegistrationChanged={onRegistrationChanged} onLoginHeroChanged={onLoginHeroChanged} onWeChatLoginChanged={onWeChatLoginChanged} />
      </SidebarInset>
    </SidebarProvider>
  );
}
