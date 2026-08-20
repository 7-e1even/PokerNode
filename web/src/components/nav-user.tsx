import { useState } from "react";
import { Bot, ExternalLink, KeyRound, LogOut } from "lucide-react";
import { AccountCredentialsDialog } from "@/components/account-credentials-dialog";
import { MCPKeyDialog } from "@/components/mcp-key-dialog";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "@/components/ui/sidebar";
import type { User } from "@/types";

export function NavUser({ user, onOpenLobby, onUserUpdated, onLogout }: { user: User; onOpenLobby: () => void; onUserUpdated: (user: User) => void; onLogout: () => void }) {
  const { isMobile } = useSidebar();
  const [accountOpen, setAccountOpen] = useState(false);
  const [mcpKeyOpen, setMCPKeyOpen] = useState(false);
  return (
    <><SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" className="data-[state=open]:bg-sidebar-accent">
              <Avatar className="size-8 rounded-lg"><AvatarImage className="rounded-lg" src={user.avatar_url} alt={user.display_name} /><AvatarFallback className="rounded-lg">{initials(user.display_name)}</AvatarFallback></Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight"><span className="truncate font-medium">{user.display_name}</span><span className="truncate text-xs text-muted-foreground">{roleLabel(user.role)}</span></div>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-(--radix-dropdown-menu-trigger-width) min-w-56" side={isMobile ? "bottom" : "right"} align="end" sideOffset={4}>
            <DropdownMenuLabel><span className="block">{user.display_name}</span><span className="block font-normal text-muted-foreground">@{user.username}</span></DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setAccountOpen(true)}><KeyRound />账号安全</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setMCPKeyOpen(true)}><Bot />Agent MCP</DropdownMenuItem>
            <DropdownMenuItem onSelect={onOpenLobby}><ExternalLink />打开玩家大厅</DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onSelect={onLogout}><LogOut />退出后台</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu><AccountCredentialsDialog user={user} open={accountOpen} onOpenChange={setAccountOpen} onUpdated={onUserUpdated} /><MCPKeyDialog open={mcpKeyOpen} onOpenChange={setMCPKeyOpen} /></>
  );
}

function initials(name: string) { return name.trim().slice(0, 2).toUpperCase(); }
function roleLabel(role: User["role"]) { return role === "super_admin" ? "超级管理员" : role === "operator" ? "运营" : "玩家"; }
