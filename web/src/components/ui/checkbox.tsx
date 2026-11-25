import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox"
import { cn } from "cn"
import { CheckIcon } from "lucide-react"

function Checkbox({ className, ...props }: CheckboxPrimitive.Root.Props) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "peer relative flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-[2px] border border-input bg-field/30 transition-colors outline-none group-has-disabled/field:opacity-50 hover:border-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-45 aria-invalid:border-destructive aria-invalid:ring-1 aria-invalid:ring-destructive/20 data-checked:border-signal data-checked:bg-signal data-checked:text-primary-foreground",
        className
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="grid place-content-center text-current transition-none [&>svg]:size-3.5 [&>svg]:stroke-[3]"
      >
        <CheckIcon
        />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

export { Checkbox }
