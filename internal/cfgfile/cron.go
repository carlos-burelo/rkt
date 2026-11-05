package cfgfile

import (
	"fmt"
	"strings"

	"rkt-proxy/internal/config"
)

// SetCronJob crea o reemplaza una tarea por nombre. No persiste: valida sobre
// una copia y publica solo si todo es válido.
func SetCronJob(cp *config.Config, appName string, job config.CronJob) error {
	work := cp.Clone()
	app := FindApp(work, appName)
	if app == nil {
		return fmt.Errorf("app %q inexistente", appName)
	}
	job.Name = strings.ToLower(strings.TrimSpace(job.Name))
	job.Schedule = strings.TrimSpace(job.Schedule)
	job.Type = strings.ToLower(strings.TrimSpace(job.Type))
	if job.Type == "" {
		job.Type = config.CronCommand
	}
	job.Command = strings.TrimSpace(job.Command)
	job.Path = strings.TrimSpace(job.Path)
	job.Method = strings.ToUpper(strings.TrimSpace(job.Method))
	if job.Type == config.CronHTTP && job.Method == "" {
		job.Method = "GET"
	}
	job.Timeout = strings.TrimSpace(job.Timeout)
	found := false
	for i := range app.Cron {
		if app.Cron[i].Name == job.Name {
			app.Cron[i] = job
			found = true
		}
	}
	if !found {
		app.Cron = append(app.Cron, job)
	}
	if err := app.ValidateApp(0); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	cp.Apps = work.Apps
	return nil
}

// DeleteCronJob elimina una tarea por nombre.
func DeleteCronJob(cp *config.Config, appName, name string) error {
	work := cp.Clone()
	app := FindApp(work, appName)
	if app == nil {
		return fmt.Errorf("app %q inexistente", appName)
	}
	out := app.Cron[:0]
	removed := false
	for _, job := range app.Cron {
		if job.Name == name {
			removed = true
			continue
		}
		out = append(out, job)
	}
	if !removed {
		return fmt.Errorf("cron %q inexistente en %s", name, appName)
	}
	app.Cron = out
	cp.Apps = work.Apps
	return nil
}
