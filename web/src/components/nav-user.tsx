import { ExternalLink, LogOut } from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "@/components/ui/sidebar";
import type { User } from "@/types";

export function NavUser({ user, onOpenLobby, onLogout }: { user: User; onOpenLobby: () => void; onLogout: () => void }) {
  const { isMobile } = useSidebar();
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" className="data-[state=open]:bg-sidebar-accent">
              <Avatar className="size-8 rounded-lg"><AvatarFallback className="rounded-lg">{initials(user.display_name)}</AvatarFallback></Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight"><span className="truncate font-medium">{user.display_name}</span><span className="truncate text-xs text-muted-foreground">{roleLabel(user.role)}</span></div>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-(--radix-dropdown-menu-trigger-width) min-w-56" side={isMobile ? "bottom" : "right"} align="end" sideOffset={4}>
            <DropdownMenuLabel><span className="block">{user.display_name}</span><span className="block font-normal text-muted-foreground">@{user.username}</span></DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onOpenLobby}><ExternalLink />打开玩家大厅</DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onSelect={onLogout}><LogOut />退出后台</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

function initials(name: string) { return name.trim().slice(0, 2).toUpperCase(); }
function roleLabel(role: User["role"]) { return role === "super_admin" ? "超级管理员" : role === "operator" ? "运营" : "玩家"; }
