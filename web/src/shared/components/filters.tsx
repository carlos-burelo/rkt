import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select, SelectContent, SelectItem, SelectTrigger,
} from "@/components/ui/select";

// Filtros compactos en dropdown: mismo poder que los grupos visibles,
// menos ruido. Un hogar para todos los filtros de la app.

// SingleFilter: una opción activa (reemplaza Segmented y grupos de
// toggles excluyentes). Muestra conteo en el item si viene.
export function SingleFilter<T extends string>({
  value,
  onChange,
  label,
  items,
}: {
  value: T;
  onChange: (v: T) => void;
  label: string;
  items: { value: T; label: string; count?: number }[];
}) {
  const cur = items.find((i) => i.value === value);
  return (
    <Select value={value} onValueChange={(v: string | null) => v != null && onChange(v as T)}>
      <SelectTrigger className="h-7 w-auto gap-1.5 font-mono text-[10px] normal-case tracking-normal" aria-label={label}>
        <span className="truncate">
          {cur ? cur.label : label}
          {cur?.count !== undefined && <span className="tnum text-muted-foreground"> · {cur.count}</span>}
        </span>
      </SelectTrigger>
      <SelectContent>
        {items.map((i) => (
          <SelectItem key={i.value} value={i.value}>
            <span>{i.label}</span>
            {i.count !== undefined && <span className="tnum text-muted-foreground"> · {i.count}</span>}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

// MultiFilter: varias opciones independientes (reemplaza grupos de
// toggles). El botón resume cuántas hay activas.
export function MultiFilter({
  label,
  items,
  onToggle,
}: {
  label: string;
  items: { key: string; label: string; checked: boolean; count?: number; disabled?: boolean }[];
  onToggle: (key: string) => void;
}) {
  const active = items.filter((i) => i.checked).length;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" size="sm" className="h-7 font-mono text-[10px] font-normal normal-case tracking-normal" aria-label={label}>
            {label} · {active}/{items.length} <ChevronDown className="h-3 w-3 opacity-60" aria-hidden />
          </Button>
        }
      />
      <DropdownMenuContent align="start" className="font-mono">
        {items.map((i) => (
          <DropdownMenuCheckboxItem
            key={i.key}
            checked={i.checked}
            disabled={i.disabled}
            onCheckedChange={() => onToggle(i.key)}
            className="text-[11px]"
          >
            {i.label}
            {i.count !== undefined && <span className="tnum ml-auto pl-4 text-muted-foreground">{i.count}</span>}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
