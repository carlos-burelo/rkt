package config

import (
	"testing"
	"time"
)

func TestCronJobValida(t *testing.T) {
	app := App{
		Name: "a", Runtime: RuntimeManaged, Command: "x", Dir: t.TempDir(), RestartDelay: "1s", MinUptime: "1s",
		Cron: []CronJob{{Name: "backup", Schedule: "*/5 * * * *", Command: "echo ok"}},
	}
	if err := app.ValidateApp(0); err != nil {
		t.Fatal(err)
	}
	if d, err := app.Cron[0].CronTimeout(); err != nil || d != 10*time.Minute {
		t.Fatalf("timeout default = %v %v", d, err)
	}
	app.Cron = []CronJob{{Name: "backup", Schedule: "bad", Command: "echo"}}
	if err := app.ValidateApp(0); err == nil {
		t.Fatal("schedule inválido")
	}
	app.Cron = []CronJob{{Name: "backup", Schedule: "* * * * *", Command: ""}}
	if err := app.ValidateApp(0); err == nil {
		t.Fatal("command vacío")
	}
	app.Cron = []CronJob{
		{Name: "x", Schedule: "* * * * *", Command: "echo"},
		{Name: "x", Schedule: "* * * * *", Command: "echo"},
	}
	if err := app.ValidateApp(0); err == nil {
		t.Fatal("nombre duplicado")
	}
	app.Cron = []CronJob{{Name: "x", Schedule: "* * * * *", Command: "echo", Timeout: "99h"}}
	if err := app.ValidateApp(0); err == nil {
		t.Fatal("timeout fuera de rango")
	}
	if ValidCronName("BAd") || !ValidCronName("backup-1") {
		t.Fatal("ValidCronName")
	}
}
