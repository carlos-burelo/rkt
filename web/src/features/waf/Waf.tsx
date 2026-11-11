import { PageHeader } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { WafShield } from "./WafShield";

// Escudo: módulo propio del WAF. Solo reglas y bloqueos del edge,
// sin mezclar con el mapa del proxy.
export function Waf() {
  const { t } = useI18n();
  return (
    <div className="space-y-4">
      <PageHeader title={t("nav.waf")} description={t("serve.waf.desc")} />
      <WafShield />
    </div>
  );
}
