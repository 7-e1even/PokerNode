import { useEffect, useRef, useState, type PointerEvent } from "react"
import { ImageIcon } from "lucide-react"

import { cn } from "@/lib/utils"
import type { LoginHeroConfig } from "@/types"

export function LoginHeroImage({ config, className, alt = "", editable = false, onPositionChange }: {
  config: LoginHeroConfig
  className?: string
  alt?: string
  editable?: boolean
  onPositionChange?: (position: { x: number; y: number }) => void
}) {
  const [failed, setFailed] = useState(false)
  const dragRef = useRef<{ clientX: number; clientY: number; positionX: number; positionY: number } | null>(null)

  useEffect(() => setFailed(false), [config.url])

  function startDrag(event: PointerEvent<HTMLDivElement>) {
    if (!editable || !config.url || !onPositionChange) return
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = { clientX: event.clientX, clientY: event.clientY, positionX: config.position_x, positionY: config.position_y }
  }

  function moveDrag(event: PointerEvent<HTMLDivElement>) {
    if (!dragRef.current || !onPositionChange) return
    const width = Math.max(event.currentTarget.clientWidth, 1)
    const height = Math.max(event.currentTarget.clientHeight, 1)
    const x = dragRef.current.positionX - ((event.clientX - dragRef.current.clientX) / width) * (100 / config.zoom)
    const y = dragRef.current.positionY - ((event.clientY - dragRef.current.clientY) / height) * (100 / config.zoom)
    onPositionChange({ x: clampPercentage(x), y: clampPercentage(y) })
  }

  function stopDrag() {
    dragRef.current = null
  }

  if (config.url && !failed) {
    return (
      <div
        className={cn("size-full overflow-hidden", editable && "touch-none cursor-grab active:cursor-grabbing", className)}
        onPointerDown={startDrag}
        onPointerMove={moveDrag}
        onPointerUp={stopDrag}
        onPointerCancel={stopDrag}
      >
        <img
          src={config.url}
          alt={alt}
          draggable={false}
          className="size-full select-none object-cover"
          style={{
            objectPosition: `${config.position_x}% ${config.position_y}%`,
            transform: `scale(${config.zoom})`,
            transformOrigin: `${config.position_x}% ${config.position_y}%`,
          }}
          onError={() => setFailed(true)}
        />
      </div>
    )
  }

  return (
    <div className={cn("relative grid size-full place-items-center overflow-hidden bg-muted", className)} aria-hidden="true">
      <div className="relative grid size-52 place-items-center text-muted-foreground/55">
        <span className="absolute inset-x-0 top-1/2 h-px -rotate-45 bg-border/70" />
        <span className="absolute inset-x-0 top-1/2 h-px rotate-45 bg-border/70" />
        <span className="absolute inset-x-6 top-1/2 h-px bg-border/70" />
        <span className="absolute inset-y-6 left-1/2 w-px bg-border/70" />
        <span className="absolute size-28 rounded-full border border-border/70" />
        <span className="absolute size-14 rounded-full border border-border/70" />
        <span className="relative grid size-12 place-items-center rounded-full border bg-muted">
          <ImageIcon className="size-5" />
        </span>
      </div>
    </div>
  )
}

function clampPercentage(value: number) {
  return Math.min(100, Math.max(0, value))
}
