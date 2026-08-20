import type { ReactNode } from "react";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";

export function SiteHeader({ title, description, actions }: { title: string; description: string; actions?: ReactNode }) {
  return (
    <header className="flex h-14 shrink-0 items-center border-b bg-background">
      <div className="flex min-w-0 flex-1 items-center gap-3 px-4 lg:px-6">
        <SidebarTrigger className="-ml-1" />
        <Separator orientation="vertical" className="h-4" />
        <div className="min-w-0"><h1 className="truncate text-sm font-semibold">{title}</h1><p className="hidden truncate text-xs text-muted-foreground sm:block">{description}</p></div>
      </div>
      {actions && <div className="shrink-0 pr-4 lg:pr-6">{actions}</div>}
    </header>
  );
}
