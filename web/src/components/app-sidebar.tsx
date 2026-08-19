import { LayoutDashboard, RadioTower, Settings2, ShieldCheck, UsersRound, WalletCards } from "lucide-react";
import { NavMain } from "@/components/nav-main";
import { NavUser } from "@/components/nav-user";
import { BrandMark } from "@/components/brand-mark";
import { Sidebar, SidebarContent, SidebarFooter, SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarRail } from "@/components/ui/sidebar";
import type { User } from "@/types";

export type AdminSection = "overview" | "users" | "roles" | "channels" | "balances" | "settings";

const items: { id: AdminSection; title: string; icon: React.ReactNode; permissions: string[] }[] = [
  { id: "overview", title: "平台总览", icon: <LayoutDashboard />, permissions: ["admin:view"] },
  { id: "users", title: "用户管理", icon: <UsersRound />, permissions: ["users:read", "users:manage"] },
  { id: "roles", title: "角色管理", icon: <ShieldCheck />, permissions: ["roles:manage"] },
  { id: "channels", title: "频道管理", icon: <RadioTower />, permissions: ["channels:manage"] },
  { id: "balances", title: "余额管理", icon: <WalletCards />, permissions: ["balances:manage"] },
  { id: "settings", title: "系统设置", icon: <Settings2 />, permissions: ["registration:manage"] },
];

export function AppSidebar({ user, active, onNavigate, onOpenLobby, onLogout }: {
  user: User;
  active: AdminSection;
  onNavigate: (section: AdminSection) => void;
  onOpenLobby: () => void;
  onLogout: () => void;
}) {
  const visibleItems = items.filter((item) => item.permissions.some((permission) => user.permissions?.includes(permission)));
  return (
    <Sidebar collapsible="icon" variant="sidebar">
      <SidebarHeader>
        <SidebarMenu><SidebarMenuItem><SidebarMenuButton size="lg" tooltip="PokerNode 运营后台" onClick={() => onNavigate("overview")}><BrandMark className="size-8" /><span className="grid flex-1 text-left leading-tight"><strong>PokerNode</strong><small className="text-muted-foreground">Operations Console</small></span></SidebarMenuButton></SidebarMenuItem></SidebarMenu>
      </SidebarHeader>
      <SidebarContent><NavMain items={visibleItems} active={active} onNavigate={onNavigate} /></SidebarContent>
      <SidebarFooter><NavUser user={user} onOpenLobby={onOpenLobby} onLogout={onLogout} /></SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
