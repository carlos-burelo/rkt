package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
)

// handleRoutes CRUD de rutas host→target. Misma semántica que el CLI:
// DELETE quita un host, PUT repunta el host existente.
func (d *Deps) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		type routeOut struct {
			Hosts  []string `json:"hosts"`
			Target string   `json:"target"`
			Path   string   `json:"path,omitempty"`
			Port   int      `json:"port,omitempty"`
		}
		out := []routeOut{}
		for _, x := range d.Live.Get().Routes {
			out = append(out, routeOut{x.Hosts, x.Target, x.Path, x.Port})
		}
		writeJSON(w, out)
	case http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		var in struct {
			Hosts  []string `json:"hosts"`
			Target string   `json:"target"`
			Path   string   `json:"path"`
			Port   int      `json:"port"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := d.mutateFleet(false, func(cp *config.Config) error {
			return cfgfile.AddRoute(cp, in.Hosts, in.Target, in.Path, in.Port)
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"ok": "true"})
	case http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		host := r.URL.Query().Get("host")
		path := r.URL.Query().Get("path")
		port := 0
		if p := r.URL.Query().Get("port"); p != "" {
			var err error
			port, err = strconv.Atoi(p)
			if err != nil || port <= 0 {
				writeErr(w, http.StatusBadRequest, "port inválido")
				return
			}
		}
		if host == "" && path == "" && port == 0 {
			writeErr(w, http.StatusBadRequest, "falta ?host= (o ?path= / ?port= para rutas sin host)")
			return
		}
		// Solo host: quirúrgico legacy (conserva el resto de hosts).
		// Con path/port: se quita la ruta entera (único camino para
		// rutas sin host).
		if path == "" && port == 0 {
			removed := false
			if err := d.mutateFleet(false, func(cp *config.Config) error {
				ok, err := cfgfile.RemoveRouteHost(cp, host)
				removed = ok
				return err
			}); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			if !removed {
				writeErr(w, http.StatusNotFound, "host no encontrado")
				return
			}
			writeJSON(w, map[string]string{"ok": "true"})
			return
		}
		dropped := 0
		if err := d.mutateFleet(false, func(cp *config.Config) error {
			n, err := cfgfile.RemoveRoutesFull(cp, host, path, port)
			dropped = n
			return err
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if dropped == 0 {
			writeErr(w, http.StatusNotFound, "ruta no encontrada")
			return
		}
		writeJSON(w, map[string]any{"ok": "true", "dropped": dropped})
	case http.MethodPut:
		if !requireOperator(w, r) {
			return
		}
		var pin struct {
			Host   string `json:"host"`
			Target string `json:"target"`
			Path   string `json:"path"`
			Port   int    `json:"port"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&pin); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if pin.Target == "" {
			writeErr(w, http.StatusBadRequest, "falta target")
			return
		}
		if pin.Host != "" && pin.Path == "" && pin.Port == 0 {
			found := false
			if err := d.mutateFleet(false, func(cp *config.Config) error {
				ok, err := cfgfile.SetRouteTarget(cp, pin.Host, pin.Target)
				found = ok
				return err
			}); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			if !found {
				writeErr(w, http.StatusNotFound, "host no encontrado")
				return
			}
			writeJSON(w, map[string]string{"ok": "true"})
			return
		}
		if pin.Host == "" && pin.Path == "" && pin.Port == 0 {
			writeErr(w, http.StatusBadRequest, "falta host, path o port")
			return
		}
		n := 0
		if err := d.mutateFleet(false, func(cp *config.Config) error {
			var err error
			n, err = cfgfile.RetargetRoutes(cp, pin.Host, pin.Path, pin.Port, pin.Target)
			return err
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if n == 0 {
			writeErr(w, http.StatusNotFound, "ruta no encontrada")
			return
		}
		writeJSON(w, map[string]any{"ok": "true", "retargeted": n})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST|PUT|DELETE")
	}
}
