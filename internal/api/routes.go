package api

import (
	"encoding/json"
	"net/http"

	"rkt-proxy/internal/cfgfile"
)

// handleRoutes CRUD de rutas host→target. Misma semántica que el CLI:
// DELETE quita un host, PUT repunta el host existente.
func (d *Deps) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		type routeOut struct {
			Hosts  []string `json:"hosts"`
			Target string   `json:"target"`
		}
		out := []routeOut{}
		for _, x := range d.Live.Get().Routes {
			out = append(out, routeOut{x.Hosts, x.Target})
		}
		writeJSON(w, out)
	case http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		var in struct {
			Hosts  []string `json:"hosts"`
			Target string   `json:"target"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		cp := d.Live.Get().Clone()
		if err := cfgfile.AddRoute(cp, in.Hosts, in.Target); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp) // hot-reload inmediato
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"ok": "true"})
	case http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		host := r.URL.Query().Get("host")
		if host == "" {
			writeErr(w, http.StatusBadRequest, "falta ?host=")
			return
		}
		cp := d.Live.Get().Clone()
		removed, err := cfgfile.RemoveRouteHost(cp, host)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if !removed {
			writeErr(w, http.StatusNotFound, "host no encontrado")
			return
		}
		if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp) // hot-reload inmediato
		writeJSON(w, map[string]string{"ok": "true"})
	case http.MethodPut:
		if !requireOperator(w, r) {
			return
		}
		var pin struct {
			Host   string `json:"host"`
			Target string `json:"target"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&pin); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		cp := d.Live.Get().Clone()
		found, err := cfgfile.SetRouteTarget(cp, pin.Host, pin.Target)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if !found {
			writeErr(w, http.StatusNotFound, "host no encontrado")
			return
		}
		if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp) // hot-reload inmediato
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST|PUT|DELETE")
	}
}
