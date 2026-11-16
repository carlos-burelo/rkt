package cli

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"rkt/internal/cfgfile"
	"rkt/internal/logx"
	"rkt/internal/pg"
	"rkt/internal/store"
)

// pgReg adapta store.DB al Registry mínimo de pg.
type pgReg struct{ db *store.DB }

func (r pgReg) PgGet(app string) (pg.Record, bool, error) {
	p, ok, err := r.db.PgGet(app)
	if err != nil || !ok {
		return pg.Record{}, ok, err
	}
	return pg.Record{App: p.App, Schema: p.Schema, Role: p.Role, Mode: p.Mode, DB: p.DB}, true, nil
}

func (r pgReg) PgList() ([]pg.Record, error) {
	all, err := r.db.PgList()
	if err != nil {
		return nil, err
	}
	out := make([]pg.Record, 0, len(all))
	for _, p := range all {
		out = append(out, pg.Record{App: p.App, Schema: p.Schema, Role: p.Role, Mode: p.Mode, DB: p.DB})
	}
	return out, nil
}

func (r pgReg) PgPut(app, schema, role, mode, db string) error {
	return r.db.PgProvision(app, schema, role, mode, db)
}

func (r pgReg) PgRemove(app string) error {
	return r.db.PgRemove(app)
}

func pgCfg(db *store.DB) pg.Config {
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		log.Fatalf("pg: %v", err)
	}
	return cfg
}

func pgCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// cmdPg gestiona la base compartida: status|list|provision|drop|backup|restore|tables|rows|query.
func cmdPg(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: pg <status|list|provision|drop|backup|restore|tables|rows|query>")
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	reg := pgReg{db}
	switch args[0] {
	case "status":
		cfg, err := pg.LoadConfig(db.GetSetting)
		if err != nil {
			fmt.Println("pg desactivado (settings set pg_mode local)")
			return
		}
		ctx, cancel := pgCtx()
		defer cancel()
		sdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, cfg.User, cfg.Pass, "postgres"))
		if err != nil {
			log.Fatalf("pg sin conexión: %v", err)
		}
		defer sdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		var ver string
		if err := sdb.QueryRowContext(ctx2, `SELECT version()`).Scan(&ver); err != nil {
			log.Fatalf("pg version: %v", err)
		}
		fmt.Printf("postgres ok: %s (db %s)\n", firstLine(ver), cfg.DB)
	case "list":
		recs, err := reg.PgList()
		if err != nil {
			log.Fatal(err)
		}
		if len(recs) == 0 {
			fmt.Println("(sin bases aprovisionadas)")
			return
		}
		for _, r := range recs {
			fmt.Printf("%-15s esquema=%-15s rol=%-15s %s\n", r.App, r.Schema, r.Role, r.Mode)
		}
	case "provision":
		var app, mode string
		for i := 0; i < len(args[1:]); i++ {
			switch args[1:][i] {
			case "--mode":
				i++
				if i >= len(args[1:]) {
					log.Fatal("uso: pg provision <app> [--mode dedicated|shared]")
				}
				mode = args[1:][i]
			default:
				if app != "" || len(args[1:][i]) == 0 || args[1:][i][0] == '-' {
					log.Fatal("uso: pg provision <app> [--mode dedicated|shared]")
				}
				app = args[1:][i]
			}
		}
		if app == "" {
			log.Fatal("uso: pg provision <app> [--mode dedicated|shared]")
		}
		if fleet, fleetDB, ferr := cfgfile.LoadMerged(cfgPath); ferr != nil {
			log.Fatalf("pg provision: %v", ferr)
		} else {
			fleetDB.Close()
			if cfgfile.FindApp(fleet, app) == nil {
				log.Fatalf("pg provision: app %q inexistente: créala primero (apps add)", app)
			}
		}
		cfg := pgCfg(db)
		ctx, cancel := pgCtx()
		defer cancel()
		url, err := pg.ProvisionApp(ctx, reg, cfg, app, mode)
		if err != nil {
			log.Fatalf("pg provision: %v", err)
		}
		if err := cfgfile.SetEnv(db, app, "DATABASE_URL", url); err != nil {
			log.Fatalf("pg provision ok pero env no: %v", err)
		}
		fmt.Printf("pg %s lista: %s\n", app, url)
	case "drop":
		if len(args) < 2 || args[1] == "" || args[1][0] == '-' {
			log.Fatal("uso: pg drop <app> [--yes]")
		}
		name := args[1]
		yes := false
		for _, a := range args[2:] {
			if a == "--yes" || a == "-y" {
				yes = true
				continue
			}
			log.Fatalf("pg drop: flag desconocido %q (uso: pg drop <app> [--yes])", a)
		}
		if !yes {
			if prompt("borrar esquema+rol de "+name+" (los datos se pierden)", "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		cfg := pgCfg(db)
		ctx, cancel := pgCtx()
		defer cancel()
		if err := pg.DropApp(ctx, reg, cfg, name); err != nil {
			log.Fatalf("pg drop: %v", err)
		}
		fmt.Printf("pg %s borrada\n", name)
	case "backup":
		if len(args) != 2 || args[1] == "" || args[1][0] == '-' {
			log.Fatal("uso: pg backup <app>")
		}
		cfg := pgCfg(db)
		ctx, cancel := pgCtx()
		defer cancel()
		outDir := filepath.Join(logx.DataDir(cfgPath), "backups")
		out, err := pg.BackupApp(ctx, cfg, reg, args[1], outDir)
		if err != nil {
			log.Fatalf("pg backup: %v", err)
		}
		fmt.Printf("backup: %s\n", out)
	case "restore":
		if len(args) < 3 || args[1] == "" || args[1][0] == '-' || args[2] == "" || args[2][0] == '-' {
			log.Fatal("uso: pg restore <app> <dump> [--yes]")
		}
		yes := false
		for _, a := range args[3:] {
			if a == "--yes" || a == "-y" {
				yes = true
				continue
			}
			log.Fatalf("pg restore: flag desconocido %q (uso: pg restore <app> <dump> [--yes])", a)
		}
		if !yes {
			if prompt("restaurar "+args[1]+" desde "+args[2]+" (sobrescribe datos)", "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		cfg := pgCfg(db)
		ctx, cancel := pgCtx()
		defer cancel()
		if err := pg.Restore(ctx, cfg.Bins.PgRestore, cfg.Host, cfg.Port, cfg.User, cfg.Pass, cfg.DB, args[2]); err != nil {
			log.Fatalf("pg restore: %v", err)
		}
		fmt.Printf("pg %s restaurada desde %s\n", args[1], args[2])
	case "tables":
		if len(args) != 2 || args[1] == "" || args[1][0] == '-' {
			log.Fatal("uso: pg tables <app>")
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg tables: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		tables, err := pg.Tables(ctx2, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg tables: %v", err)
		}
		for _, t := range tables {
			fmt.Println(t)
		}
	case "rows":
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			log.Fatal("uso: pg rows <app> <tabla> [--n 100] [--page 1]")
		}
		n, page := 100, 1
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--n":
				i++
				fmt.Sscanf(args[i], "%d", &n)
			case "--page":
				i++
				fmt.Sscanf(args[i], "%d", &page)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg rows: %v", err)
		}
		defer cdb.Close()
		cols, rows, total, err := pg.TableRows(ctx, cdb, rec.Schema, args[2], n, (page-1)*n)
		if err != nil {
			log.Fatalf("pg rows: %v", err)
		}
		byName := map[string]string{}
		if cis, cerr := pg.ColumnTypes(ctx, cdb, rec.Schema, args[2]); cerr == nil {
			for _, c := range cis {
				byName[c.Name] = c.Type
			}
		}
		head := make([]string, len(cols))
		for i, c := range cols {
			if t := byName[c]; t != "" {
				head[i] = c + ":" + t
			} else {
				head[i] = c
			}
		}
		fmt.Println(strings.Join(head, "\t"))
		for _, r := range rows {
			cells := make([]string, len(r))
			for i, v := range r {
				cells[i] = fmt.Sprint(v)
			}
			fmt.Println(strings.Join(cells, "\t"))
		}
		fmt.Printf("(%d filas, total %d, pág %d)\n", len(rows), total, page)
	case "query":
		if len(args) < 3 || args[1] == "" {
			log.Fatal("uso: pg query <app> <sql> [--yes]")
		}
		yes := false
		parts := args[2:]
		var sqlParts []string
		for _, a := range parts {
			if a == "--yes" || a == "-y" {
				yes = true
				continue
			}
			sqlParts = append(sqlParts, a)
		}
		sqlText := strings.Join(sqlParts, " ")
		if strings.TrimSpace(sqlText) == "" {
			log.Fatal("uso: pg query <app> <sql> [--yes]")
		}
		if !yes {
			if prompt("ejecutar en "+args[1]+": "+sqlText, "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg query: %v", err)
		}
		defer cdb.Close()
		res, err := pg.Query(ctx, cdb, sqlText)
		if err != nil {
			log.Fatalf("pg query: %v", err)
		}
		if res.Command == "SELECT" {
			fmt.Println(strings.Join(res.Columns, "\t"))
			for _, r := range res.Rows {
				cells := make([]string, len(r))
				for i, v := range r {
					cells[i] = fmt.Sprint(v)
				}
				fmt.Println(strings.Join(cells, "\t"))
			}
		} else {
			fmt.Printf("ok, afectadas %d\n", res.Affected)
		}
	default:
		log.Fatal("uso: pg <status|list|provision|drop|backup|restore|tables|rows|query>")
	}
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' || c == '\r' {
			return s[:i]
		}
	}
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
