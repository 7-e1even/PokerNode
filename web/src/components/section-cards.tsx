import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import type { AdminOverview } from "@/types";

export function SectionCards({ overview }: { overview: AdminOverview }) {
  const bound = overview.platform_counts.bound_memberships;
  const memberships = overview.platform_counts.memberships;
  const bindingRate = memberships === 0 ? 0 : Math.round((bound / memberships) * 100);
  const metrics = [
    { label: "平台账号", value: overview.counts.total || 0, badge: `${overview.counts.active || 0} 正常`, note: "独立 PokerNode 账号" },
    { label: "New API 频道", value: overview.platform_counts.spaces, badge: `${overview.platform_counts.tables} 桌`, note: "每个频道对应一个节点" },
    { label: "频道成员", value: memberships, badge: `${bindingRate}% 已绑定`, note: `${bound} 个 New API 凭证有效` },
    { label: "资金流水", value: overview.platform_counts.operations, badge: overview.platform_counts.failed_operations ? `${overview.platform_counts.failed_operations} 失败` : "全部正常", note: "买入与离桌结算记录" },
  ];
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      {metrics.map((metric) => <Card key={metric.label}><CardHeader><CardDescription>{metric.label}</CardDescription><CardTitle className="text-3xl tabular-nums">{metric.value}</CardTitle><CardAction><Badge variant="outline">{metric.badge}</Badge></CardAction></CardHeader><CardFooter className="text-xs text-muted-foreground">{metric.note}</CardFooter></Card>)}
    </div>
  );
}
