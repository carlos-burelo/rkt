import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { es } from "./messages/es";

export type Locale = "es";
export type Messages = Record<string, string>;
export type Vars = Record<string, string | number>;

const DICTS: Record<Locale, Messages> = { es };
export const DEFAULT_LOCALE: Locale = "es";
export const LOCALES: { id: Locale; label: string }[] = [
  { id: "es", label: "Español" },
];

let activeLocale: Locale = DEFAULT_LOCALE;

export function getLocale(): Locale {
  return activeLocale;
}

function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, key: string) =>
    key in vars ? String(vars[key]) : `{${key}}`,
  );
}

// t clave→texto con interpolación {var}. La clave ausente se muestra tal cual
// para detectar strings sin migrar en desarrollo.
export function t(key: string, vars?: Vars): string {
  return interpolate(DICTS[activeLocale][key] ?? key, vars);
}

// tn pluraliza con Intl.PluralRules: clave.one / clave.other (y zero, many…).
export function tn(key: string, n: number, vars?: Vars): string {
  const cat = new Intl.PluralRules(activeLocale).select(n);
  const template = DICTS[activeLocale][`${key}.${cat}`] ?? DICTS[activeLocale][`${key}.other`] ?? key;
  return interpolate(template, { n, ...vars });
}

type I18nValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: typeof t;
  tn: typeof tn;
};

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(DEFAULT_LOCALE);
  const setLocale = useCallback((next: Locale) => {
    activeLocale = next;
    setLocaleState(next);
  }, []);
  const value = useMemo<I18nValue>(
    () => ({ locale, setLocale, t, tn }),
    [locale, setLocale],
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n fuera de I18nProvider");
  return ctx;
}
