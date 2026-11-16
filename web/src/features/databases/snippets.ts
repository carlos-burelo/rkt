// Consultas guardadas e historial: viven en el navegador (localStorage),
// por app. No tocan el servidor: son comodidad del editor, no datos.

export interface SavedQuery {
  id: string;
  name: string;
  sql: string;
  ts: number;
}

export interface HistEntry {
  sql: string;
  ts: number;
}

const savedKey = (app: string) => `rkt.sql.saved.${app}`;
const histKey = (app: string) => `rkt.sql.hist.${app}`;

function read<T>(k: string, fb: T): T {
  try {
    const raw = localStorage.getItem(k);
    if (!raw) return fb;
    return JSON.parse(raw) as T;
  } catch {
    return fb;
  }
}

function write(k: string, v: unknown) {
  try {
    localStorage.setItem(k, JSON.stringify(v));
  } catch {
    // cuota llena o sin storage: el editor sigue funcionando
  }
}

export function loadSaved(app: string): SavedQuery[] {
  return read<SavedQuery[]>(savedKey(app), []);
}

export function saveQuery(app: string, name: string, sql: string): SavedQuery[] {
  const q: SavedQuery = {
    id: `${Date.now().toString(36)}${Math.floor(Math.random() * 1e6).toString(36)}`,
    name: name.trim() || sql.split("\n")[0].slice(0, 40) || "sin título",
    sql,
    ts: Date.now(),
  };
  const next = [q, ...loadSaved(app)].slice(0, 50);
  write(savedKey(app), next);
  return next;
}

export function deleteSaved(app: string, id: string): SavedQuery[] {
  const next = loadSaved(app).filter((q) => q.id !== id);
  write(savedKey(app), next);
  return next;
}

export function loadHist(app: string): HistEntry[] {
  return read<HistEntry[]>(histKey(app), []);
}

// pushHist guarda en historial (cap 20, sin duplicados seguidos).
export function pushHist(app: string, sql: string): HistEntry[] {
  const prev = loadHist(app);
  if (prev[0]?.sql === sql) return prev;
  const next = [{ sql, ts: Date.now() }, ...prev].slice(0, 20);
  write(histKey(app), next);
  return next;
}
