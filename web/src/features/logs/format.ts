// Formato compacto de logs estructurados (JSON/logfmt) sin perder el raw.
export type Level = "info" | "warning" | "error";

export interface LogField {
  key: string;
  value: string;
}

export interface ParsedLine {
  raw: string;
  structured: boolean;
  time: string;
  timestamp?: number;
  level: Level;
  msg: string;
  fields: LogField[];
}

const TIME_KEYS = /^(time|ts|timestamp|date|@timestamp)$/i;
const LEVEL_KEYS = /^(level|severity|lvl|log\.level)$/i;
const MSG_KEYS = /^(msg|message|event|log)$/i;

function levelFrom(value: string): Level {
  const v = value.toLowerCase();
  if (v.startsWith("err") || v === "fatal" || v === "panic") return "error";
  if (v.startsWith("warn")) return "warning";
  return "info";
}

function normalizeLevel(value: unknown): Level | undefined {
  if (typeof value !== "string") return undefined;
  if (!value.trim()) return undefined;
  return levelFrom(value.trim());
}

export function levelOf(raw: string): Level {
  const parsed = parseLogLine(raw);
  if (parsed.structured) return parsed.level;
  if (/\b(ERROR|FATAL|PANIC|error=|level=error)\b/.test(raw)) return "error";
  if (/\b(WARN|WARNING|warn=|level=warn)\b/.test(raw)) return "warning";
  return "info";
}

function valueString(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function timeInfo(value: string): { label: string; ts?: number } {
  const raw = value.trim();
  if (!raw) return { label: "" };
  if (/^\d{2}:\d{2}:\d{2}/.test(raw)) return { label: raw.slice(0, 8) };
  const numeric = Number(raw);
  const ms = Number.isFinite(numeric) && /^\d+(?:\.\d+)?$/.test(raw)
    ? (numeric > 1e11 ? numeric : numeric * 1000)
    : new Date(raw).getTime();
  if (Number.isNaN(ms)) return { label: raw.slice(0, 12) };
  return {
    label: new Date(ms).toLocaleTimeString("es", { hour12: false }),
    ts: ms,
  };
}

function parseJSON(raw: string): ParsedLine | null {
  const trimmed = raw.trim();
  if (!trimmed.startsWith("{") || !trimmed.endsWith("}")) return null;
  let obj: Record<string, unknown>;
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    obj = parsed as Record<string, unknown>;
  } catch {
    return null;
  }
  const keys = Object.keys(obj);
  if (keys.length === 0) return null;
  const timeKey = keys.find((k) => TIME_KEYS.test(k));
  const levelKey = keys.find((k) => LEVEL_KEYS.test(k));
  const msgKey = keys.find((k) => MSG_KEYS.test(k));
  const time = timeKey ? timeInfo(valueString(obj[timeKey])) : { label: "" };
  const level = levelKey ? (normalizeLevel(obj[levelKey]) ?? "info") : "info";
  const fields = keys
    .filter((k) => k !== timeKey && k !== levelKey && k !== msgKey)
    .map((k) => ({ key: k, value: valueString(obj[k]) }));
  return {
    raw,
    structured: true,
    time: time.label,
    timestamp: time.ts,
    level,
    msg: msgKey ? valueString(obj[msgKey]) : "",
    fields,
  };
}

function parseLogfmt(raw: string): ParsedLine | null {
  const re = /([A-Za-z_][\w.-]*)=(?:"((?:[^"\\]|\\.)*)"|'([^']*)'|(\S+))/g;
  const fields: LogField[] = [];
  let match: RegExpExecArray | null;
  while ((match = re.exec(raw)) !== null) {
    fields.push({ key: match[1], value: match[2] ?? match[3] ?? match[4] ?? "" });
  }
  if (fields.length === 0) return null;

  const prefix = /^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?)\s+/.exec(raw);
  const timeField = fields.find((f) => TIME_KEYS.test(f.key));
  const time = timeInfo(timeField?.value ?? prefix?.[1] ?? "");
  const levelField = fields.find((f) => LEVEL_KEYS.test(f.key));
  const level = normalizeLevel(levelField?.value) ?? "info";
  const msgField = fields.find((f) => MSG_KEYS.test(f.key));
  const rest = fields.filter((f) => !TIME_KEYS.test(f.key) && !LEVEL_KEYS.test(f.key) && !MSG_KEYS.test(f.key));
  // Texto libre entre campos: "time=... level=INFO checkout: request ok path=..."
  let msg = msgField?.value ?? "";
  if (!msgField) {
    msg = raw.replace(re, " ");
    if (prefix) msg = msg.replace(prefix[1], " ");
    msg = msg.replace(/\s+/g, " ").trim();
  }
  const structured = fields.length >= 2 || !!timeField || !!levelField || !!msgField;

  return {
    raw,
    structured,
    time: time.label,
    timestamp: time.ts,
    level,
    msg,
    fields: rest,
  };
}

function parsePlain(raw: string): ParsedLine {
  const iso = /^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?)\s+(?:(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL|PANIC)\b\s*)?(.*)$/i.exec(raw);
  if (iso) {
    const time = timeInfo(iso[1]);
    return {
      raw,
      structured: true,
      time: time.label,
      timestamp: time.ts,
      level: levelFrom(iso[2] ?? "info"),
      msg: iso[3] ?? "",
      fields: [],
    };
  }
  const level = /^(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL|PANIC)\b\s*(.*)$/i.exec(raw);
  if (level) {
    return {
      raw,
      structured: true,
      time: "",
      level: levelFrom(level[1]),
      msg: level[2] ?? "",
      fields: [],
    };
  }
  return { raw, structured: false, time: "", level: "info", msg: raw, fields: [] };
}

export function parseLogLine(raw: string): ParsedLine {
  return parseJSON(raw) ?? parseLogfmt(raw) ?? parsePlain(raw);
}

export interface GroupedLine {
  line: ParsedLine;
  count: number;
}

// Agrupa líneas consecutivas con el mismo mensaje/campos. Conserva el raw
// más reciente y el conteo para comprimir ráfagas repetidas.
export function groupParsed(lines: ParsedLine[]): GroupedLine[] {
  const out: { line: ParsedLine; count: number; sig: string }[] = [];
  for (const line of lines) {
    const sig = line.structured
      ? JSON.stringify([line.level, line.msg, line.fields.map((f) => [f.key, f.value])])
      : `raw:${line.raw}`;
    const last = out[out.length - 1];
    if (last && last.sig === sig) {
      last.line = line;
      last.count++;
    } else {
      out.push({ line, count: 1, sig });
    }
  }
  return out.map(({ line, count }) => ({ line, count }));
}
