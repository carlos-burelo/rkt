package cfgfile

import (
	"testing"

	"rkt-proxy/internal/config"
)

func TestSetCronJobHTTPNormaliza(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{{
		Name: "a", Runtime: config.RuntimeManaged, Command: "x",
		Dir: t.TempDir(), RestartDelay: "1s", MinUptime: "1s",
	}}}
	if err := SetCronJob(cfg, "a", config.CronJob{
		Name: " ping ", Schedule: "* * * * *", Type: "HTTP", Path: " /health ",
	}); err != nil {
		t.Fatal(err)
	}
	job := cfg.Apps[0].Cron[0]
	if job.Name != "ping" || job.Type != config.CronHTTP || job.Path != "/health" || job.Method != "GET" {
		t.Fatalf("job = %+v", job)
	}
	// Inválido no publica la copia.
	if err := SetCronJob(cfg, "a", config.CronJob{
		Name: "roto", Schedule: "* * * * *", Type: config.CronHTTP, Path: "",
	}); err == nil {
		t.Fatal("http sin path debía fallar")
	}
	if len(cfg.Apps[0].Cron) != 1 {
		t.Fatalf("mutación parcial: %+v", cfg.Apps[0].Cron)
	}
}
