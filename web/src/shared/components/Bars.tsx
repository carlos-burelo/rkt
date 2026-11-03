// Bars sparkline sin libs (divs proporcionales).
export function Bars({ data, height = 64 }: { data: number[]; height?: number }) {
  const max = Math.max(1, ...data);
  return (
    <div className="flex items-end gap-0.5" style={{ height }}>
      {data.map((v, i) => (
        <div
          key={i}
          className="min-w-1 flex-1 rounded-sm bg-zinc-600"
          style={{ height: `${Math.max(4, (v / max) * 100)}%` }}
          title={`${v}`}
        />
      ))}
    </div>
  );
}
