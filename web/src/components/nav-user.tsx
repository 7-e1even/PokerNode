import { ExternalLink, LogOut, Settings2 } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import type { User } from "@/types";

export function NavUser({ user, onOpenLobby, onOpenSettings, onLogout }: { user: User; onOpenLobby: () => void; onOpenSettings: () => void; onLogout: () => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="h-auto gap-2 p-1.5 pr-2">
          <Avatar className="size-8"><AvatarImage src={user.avatar_url} alt={user.display_name} /><AvatarFallback>{initials(user.display_name)}</AvatarFallback></Avatar>
          <span className="hidden min-w-0 text-left sm:block"><strong className="block max-w-32 truncate text-xs">{user.display_name}</strong><small className="block text-xs text-muted-foreground">{roleLabel(user.role)}</small></span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-56" align="end">
        <DropdownMenuLabel><span className="block">{user.display_name}</span><span className="block font-normal text-muted-foreground">@{user.username}</span></DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onOpenSettings}><Settings2 />个人设置</DropdownMenuItem>
        <DropdownMenuItem onSelect={onOpenLobby}><ExternalLink />打开玩家大厅</DropdownMenuItem>
        <DropdownMenuItem variant="destructive" onSelect={onLogout}><LogOut />退出后台</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function initials(name: string) { return name.trim().slice(0, 2).toUpperCase(); }
function roleLabel(role: User["role"]) { return role === "super_admin" ? "超级管理员" : role === "operator" ? "运营" : "玩家"; }
