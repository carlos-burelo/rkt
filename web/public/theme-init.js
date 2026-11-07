// Aplica el tema antes del primer paint (sin flash).
// Archivo externo (no inline) para respetar el CSP script-src 'self'
// del dashboard. Debe cargarse sin defer/async en <head>.
(function () {
  try {
    var t = localStorage.getItem("rkt-theme") || "system";
    var d = t === "dark" || (t === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
    var r = document.documentElement;
    r.classList.toggle("dark", d);
    r.classList.toggle("light", !d);
    r.style.colorScheme = d ? "dark" : "light";
  } catch (e) {}
})();
