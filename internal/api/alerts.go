package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"rkt-proxy/internal/alerts"
	"rkt-proxy/internal/cfgfile"
)

// handleAlerts lista (viewer) o crea (operator).
func (d *Deps) handleAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := d.DB.ListRules()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, rules)
	case http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		var in alertIn
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := d.validateAlert(in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, err := d.DB.AddRuleFull(in.App, in.Kind, in.Threshold, in.WindowMin, in.Enabled == nil || *in.Enabled, in.ChannelID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]int64{"id": id})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	}
}

type alertIn struct {
	App       string  `json:"app"`
	Kind      string  `json:"kind"`
	Threshold float64 `json:"threshold"`
	WindowMin int     `json:"window_min"`
	Enabled   *bool   `json:"enabled"`
	ChannelID int64   `json:"channel_id"`
}

func (d *Deps) validateAlert(in alertIn) error {
	if in.App != "" && in.App != "*" && cfgfile.FindApp(d.Live.Get(), in.App) == nil {
		return fmt.Errorf("app inexistente: %s", in.App)
	}
	if in.ChannelID > 0 {
		chans, err := d.DB.ListChannels()
		if err != nil {
			return err
		}
		found := false
		for _, c := range chans {
			if c.ID == in.ChannelID {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("canal #%d inexistente", in.ChannelID)
		}
	}
	return nil
}

// handleAlert enruta /alerts/{id}, /alerts/{id}/test y /alerts/{id}/fires.
func (d *Deps) handleAlert(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/alerts/"), "/")
	parts := strings.Split(rest, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodPut:
		if !requireOperator(w, r) {
			return
		}
		var in alertIn
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := d.validateAlert(in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.DB.UpdateRule(id, in.App, in.Kind, in.Threshold, in.WindowMin, in.Enabled == nil || *in.Enabled, in.ChannelID); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "" && r.Method == http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		if err := d.DB.DeleteRule(id); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "test" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		rules, err := d.DB.ListRules()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, rule := range rules {
			if rule.ID == id {
				writeJSON(w, map[string]any{"ok": true, "message": alerts.Describe(rule, alerts.Metrics{})})
				return
			}
		}
		writeErr(w, http.StatusNotFound, "regla inexistente")
	case action == "fires" && r.Method == http.MethodGet:
		fires, err := d.DB.ListFires(id, 50)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, fires)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "PUT|DELETE|POST test|GET fires")
	}
}

// handleAlertChannels CRUD de canales webhook.
func (d *Deps) handleAlertChannels(w http.ResponseWriter, r *http.Request) {
	if !requireOperator(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := d.DB.ListChannels()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, list)
	case http.MethodPost:
		var in struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := validWebhook(in.URL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, err := d.DB.AddChannel(strings.TrimSpace(in.Name), strings.TrimSpace(in.URL), in.Enabled == nil || *in.Enabled)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]int64{"id": id})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	}
}

// handleAlertChannel edita o borra un canal por id.
func (d *Deps) handleAlertChannel(w http.ResponseWriter, r *http.Request) {
	if !requireOperator(w, r) {
		return
	}
	id, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/alert-channels/"), "/"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var in struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := validWebhook(in.URL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.DB.UpdateChannel(id, strings.TrimSpace(in.Name), strings.TrimSpace(in.URL), in.Enabled == nil || *in.Enabled); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case http.MethodDelete:
		if err := d.DB.DeleteChannel(id); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "PUT|DELETE")
	}
}

func validWebhook(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("webhook inválido: usa http(s)://host/ruta")
	}
	return nil
}
