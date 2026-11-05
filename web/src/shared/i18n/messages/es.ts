import { common } from "./es/common";
import { create } from "./es/create";
import { nav } from "./es/nav";
import { shell } from "./es/shell";
import { state } from "./es/state";

// Español neutro: sin voseo ni regionalismos. Cada dominio en su archivo.
export const es: Record<string, string> = {
  ...common,
  ...nav,
  ...state,
  ...shell,
  ...create,
};
