import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart";
import type { AdminSpaceSummary } from "@/types";

const chartConfig = {
  members: { label: "频道成员", color: "var(--chart-1)" },
  operations: { label: "资金流水", color: "var(--chart-2)" },
} satisfies ChartConfig;

export function ChartAreaInteractive({ spaces }: { spaces: AdminSpaceSummary[] }) {
  const data = spaces.slice(0, 8).reverse().map((space) => ({ name: space.name, members: space.member_count, operations: space.operation_count }));
  return (
    <Card>
      <CardHeader><CardTitle>频道规模</CardTitle><CardDescription>最近 8 个频道的成员和资金流水数量</CardDescription></CardHeader>
      <CardContent>
        {data.length === 0 ? <div className="grid h-60 place-items-center text-sm text-muted-foreground">创建频道后会在这里显示真实运行数据</div> : (
          <ChartContainer config={chartConfig} className="h-64 w-full">
            <AreaChart data={data} margin={{ left: -16, right: 12 }}><defs><linearGradient id="fillMembers" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="var(--color-members)" stopOpacity={0.45} /><stop offset="95%" stopColor="var(--color-members)" stopOpacity={0.04} /></linearGradient><linearGradient id="fillOperations" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="var(--color-operations)" stopOpacity={0.35} /><stop offset="95%" stopColor="var(--color-operations)" stopOpacity={0.03} /></linearGradient></defs><CartesianGrid vertical={false} /><XAxis dataKey="name" tickLine={false} axisLine={false} tickMargin={8} tickFormatter={(value) => String(value).slice(0, 6)} /><YAxis allowDecimals={false} tickLine={false} axisLine={false} /><ChartTooltip content={<ChartTooltipContent indicator="dot" />} /><Area dataKey="operations" type="monotone" fill="url(#fillOperations)" stroke="var(--color-operations)" /><Area dataKey="members" type="monotone" fill="url(#fillMembers)" stroke="var(--color-members)" /></AreaChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}
