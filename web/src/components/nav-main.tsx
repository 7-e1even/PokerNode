import type { ReactNode } from "react";
import { SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar";

export function NavMain<T extends string>({ items, active, onNavigate }: {
  items: { id: T; title: string; icon: ReactNode }[];
  active: T;
  onNavigate: (id: T) => void;
}) {
  return (
    <SidebarGroup>
      <SidebarGroupLabel>运营管理</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => (
            <SidebarMenuItem key={item.id}>
              <SidebarMenuButton tooltip={item.title} isActive={active === item.id} onClick={() => onNavigate(item.id)}>
                {item.icon}<span>{item.title}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
