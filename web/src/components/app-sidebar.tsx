import { LayoutDashboard, RadioTower, Settings2, ShieldCheck, Trophy, UsersRound, WalletCards } from "lucide-react";
import { NavMain } from "@/components/nav-main";
import { BrandMark } from "@/components/brand-mark";
import { Sidebar, SidebarContent, SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarRail } from "@/components/ui/sidebar";
import type { User } from "@/types";

export type AdminSection = "overview" | "users" | "roles" | "channels" | "balances" | "rankings" | "settings";

const items: { id: AdminSection; title: string; icon: React.ReactNode; permissions: string[] }[] = [
  { id: "overview", title: "平台总览", icon: <LayoutDashboard />, permissions: ["admin:view"] },
  { id: "users", title: "用户管理", icon: <UsersRound />, permissions: ["users:read", "users:manage"] },
  { id: "roles", title: "角色管理", icon: <ShieldCheck />, permissions: ["roles:manage"] },
  { id: "channels", title: "频道管理", icon: <RadioTower />, permissions: ["channels:manage"] },
  { id: "balances", title: "余额管理", icon: <WalletCards />, permissions: ["balances:manage"] },
  { id: "rankings", title: "排名管理", icon: <Trophy />, permissions: ["rankings:manage"] },
  { id: "settings", title: "系统设置", icon: <Settings2 />, permissions: ["registration:manage", "auth_settings:manage", "branding:manage"] },
];

export function AppSidebar({ user, siteName, active, onNavigate }: {
  user: User;
  siteName: string;
  active: AdminSection;
  onNavigate: (section: AdminSection) => void;
}) {
  const visibleItems = items.filter((item) => item.permissions.some((permission) => user.permissions?.includes(permission)));
  return (
    <Sidebar collapsible="icon" variant="sidebar">
      <SidebarHeader>
        <SidebarMenu><SidebarMenuItem><SidebarMenuButton size="lg" tooltip={`${siteName} 运营后台`} onClick={() => onNavigate("overview")}><BrandMark className="size-8" /><span className="grid flex-1 text-left leading-tight"><strong>{siteName}</strong><small className="text-muted-foreground">Operations Console</small></span></SidebarMenuButton></SidebarMenuItem></SidebarMenu>
      </SidebarHeader>
      <SidebarContent><NavMain items={visibleItems} active={active} onNavigate={onNavigate} /></SidebarContent>
      <SidebarRail />
    </Sidebar>
  );
}
