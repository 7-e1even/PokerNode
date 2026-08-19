import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";
import linkedCardsLogo from "@/assets/brand/pokernode-linked-cards-porcelain.svg";

export function BrandMark({ className, alt = "", ...props }: ComponentProps<"img">) {
  return <img src={linkedCardsLogo} alt={alt} className={cn("shrink-0", className)} {...props} />;
}
