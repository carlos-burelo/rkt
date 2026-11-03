import { mergeProps } from "@base-ui/react/merge-props"
import { useRender } from "@base-ui/react/use-render"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"

const badgeVariants = cva(
  "group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-[2px] border px-1.5 py-0.5 font-mono text-[10px] leading-none font-medium tracking-[0.08em] whitespace-nowrap uppercase transition-colors focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 has-data-[icon=inline-end]:pr-1 has-data-[icon=inline-start]:pl-1 [&>svg]:pointer-events-none [&>svg]:size-3!",
  {
    variants: {
      variant: {
        default:
          "border-signal/35 bg-signal/12 text-signal [a]:hover:bg-signal/18",
        secondary:
          "border-border bg-secondary/70 text-foreground [a]:hover:bg-secondary",
        destructive:
          "border-bad/35 bg-bad/10 text-bad focus-visible:ring-destructive/20 [a]:hover:bg-bad/15",
        outline:
          "border-border bg-transparent text-muted-foreground [a]:hover:border-input [a]:hover:text-foreground",
        ghost:
          "border-transparent bg-transparent text-muted-foreground hover:text-foreground",
        link: "border-transparent bg-transparent text-signal underline-offset-4 hover:underline",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

function Badge({
  className,
  variant = "default",
  render,
  ...props
}: useRender.ComponentProps<"span"> & VariantProps<typeof badgeVariants>) {
  return useRender({
    defaultTagName: "span",
    props: mergeProps<"span">(
      {
        className: cn(badgeVariants({ variant }), className),
      },
      props
    ),
    render,
    state: {
      slot: "badge",
      variant,
    },
  })
}

export { Badge, badgeVariants }
