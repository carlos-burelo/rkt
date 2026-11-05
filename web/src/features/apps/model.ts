// Reglas puras de la ficha de app (testeables sin DOM).
export function canSaveApp(command: string, external: boolean, staticDir?: string): boolean {
  if (external || staticDir) return true;
  return command.trim() !== "";
}

export function normalizeDomain(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/.*$/, "")
    .replace(/\.$/, "");
}
