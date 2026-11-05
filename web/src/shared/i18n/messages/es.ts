import { apps } from "./es/apps";
import { common } from "./es/common";
import { create } from "./es/create";
import { deployments } from "./es/deployments";
import { nav } from "./es/nav";
import { ops } from "./es/ops";
import { overview } from "./es/overview";
import { serve } from "./es/serve";
import { shared } from "./es/shared";
import { shell } from "./es/shell";
import { state } from "./es/state";
import { system } from "./es/system";

// Español neutro: sin voseo ni regionalismos. Cada dominio en su archivo.
export const es: Record<string, string> = {
  ...common,
  ...nav,
  ...state,
  ...shell,
  ...create,
  ...overview,
  ...deployments,
  ...apps,
  ...ops,
  ...serve,
  ...system,
  ...shared,
};
