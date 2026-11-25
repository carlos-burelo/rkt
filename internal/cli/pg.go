package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
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

// cmdPg gestiona la base compartida: status|list|provision|drop|backup|restore|tables|rows|schema|insert|update|delete|create-table|add-column|drop-table|drop-column|query|indexes|create-index|drop-index|constraints|policies|create-policy|drop-policy|rename-table|rename-column|alter-column|truncate|duplicate|backups|backup-delete|rows-export|rows-import|explain|views|create-view|drop-view|matview-refresh|functions|drop-function|procedures|proc-call|drop-procedure|triggers|create-trigger|drop-trigger|trigger-toggle|sequences|seq-restart|seq-setval.
func cmdPg(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: pg <status|list|provision|drop|backup|restore|tables|rows|schema|insert|update|delete|create-table|add-column|drop-table|drop-column|query|indexes|create-index|drop-index|constraints|policies|create-policy|drop-policy|rename-table|rename-column|alter-column|truncate|duplicate|backups|backup-delete|rows-export|rows-import|explain|views|create-view|drop-view|matview-refresh|functions|drop-function|procedures|proc-call|drop-procedure|triggers|create-trigger|drop-trigger|trigger-toggle|sequences|seq-restart|seq-setval>")
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
			log.Fatal("uso: pg rows <app> <tabla> [--n 100] [--page 1] [--order col] [--dir asc|desc] [--filter col:op:val]... [--search q]")
		}
		n, page := 100, 1
		var order, dir, search string
		var rawFilters []string
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--n":
				i++
				fmt.Sscanf(args[i], "%d", &n)
			case "--page":
				i++
				fmt.Sscanf(args[i], "%d", &page)
			case "--order":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows <app> <tabla> [--order col]")
				}
				order = args[i]
			case "--dir":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows <app> <tabla> [--dir asc|desc]")
				}
				dir = args[i]
			case "--filter":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows <app> <tabla> [--filter col:op:val]")
				}
				rawFilters = append(rawFilters, args[i])
			case "--search":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows <app> <tabla> [--search q]")
				}
				search = args[i]
			default:
				log.Fatalf("pg rows: flag desconocido %q", args[i])
			}
		}
		filters, err := pg.ParseFilters(rawFilters)
		if err != nil {
			log.Fatalf("pg rows: %v", err)
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
		cols, rows, total, err := pg.TableRowsEx(ctx, cdb, rec.Schema, args[2], pg.RowsQuery{
			Limit: n, Offset: (page - 1) * n,
			OrderCol: order, OrderDir: dir, Filters: filters, Search: search,
		})
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
			// Multi-sentencia separada por ; (una sola mantiene el formato actual).
			if stmts := pg.SplitStatements(sqlText); len(stmts) > 1 {
				results, merr := pg.QueryMulti(ctx, cdb, sqlText)
				if merr != nil {
					log.Fatalf("pg query: %v", merr)
				}
				for i, mr := range results {
					fmt.Printf("--- stmt %d (%s)\n", i+1, mr.Command)
					if mr.Command == "SELECT" {
						fmt.Println(strings.Join(mr.Columns, "\t"))
						for _, r := range mr.Rows {
							cells := make([]string, len(r))
							for j, v := range r {
								cells[j] = fmt.Sprint(v)
							}
							fmt.Println(strings.Join(cells, "\t"))
						}
					} else {
						fmt.Printf("ok, afectadas %d\n", mr.Affected)
					}
				}
				break
			}
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
	case "schema":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg schema <app> [tabla]")
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
			log.Fatalf("pg schema: %v", err)
		}
		defer cdb.Close()
		if len(args) == 3 {
			s, err := pg.DescribeTable(ctx, cdb, rec.Schema, args[2])
			if err != nil {
				log.Fatalf("pg schema: %v", err)
			}
			printSchema(s)
			pols, err := pg.ListPolicies(ctx, cdb, rec.Schema, args[2])
			if err != nil {
				log.Fatalf("pg schema: %v", err)
			}
			for _, p := range pols {
				fmt.Printf("policy %s %s roles=%s\n", p.Name, p.Command, p.Roles)
			}
			break
		}
		tables, err := pg.Tables(ctx, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg schema: %v", err)
		}
		for _, t := range tables {
			s, err := pg.DescribeTable(ctx, cdb, rec.Schema, t)
			if err != nil {
				log.Fatalf("pg schema: %v", err)
			}
			printSchema(s)
		}
	case "insert":
		if len(args) < 4 || args[1] == "" || args[2] == "" || args[3] == "" {
			log.Fatal(`uso: pg insert <app> <tabla> '{"col":valor}' [--yes]`)
		}
		yes := hasYes(args[4:])
		data := mustJSONMap(args[3])
		if !yes {
			if prompt(fmt.Sprintf("insertar en %s.%s: %s", args[1], args[2], args[3]), "no") != "si" {
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
			log.Fatalf("pg insert: %v", err)
		}
		defer cdb.Close()
		row, err := pg.InsertRow(ctx, cdb, rec.Schema, args[2], data)
		if err != nil {
			log.Fatalf("pg insert: %v", err)
		}
		fmt.Printf("ok: %v\n", row)
	case "update":
		if len(args) < 5 || args[1] == "" || args[2] == "" || args[3] == "" || args[4] == "" {
			log.Fatal(`uso: pg update <app> <tabla> '<pk-json>' '<patch-json>' [--yes]`)
		}
		yes := hasYes(args[5:])
		pk, patch := mustJSONMap(args[3]), mustJSONMap(args[4])
		if !yes {
			if prompt(fmt.Sprintf("actualizar %s.%s donde %s: %s", args[1], args[2], args[3], args[4]), "no") != "si" {
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
			log.Fatalf("pg update: %v", err)
		}
		defer cdb.Close()
		n, err := pg.UpdateRow(ctx, cdb, rec.Schema, args[2], pk, patch)
		if err != nil {
			log.Fatalf("pg update: %v", err)
		}
		fmt.Printf("ok, actualizadas %d\n", n)
	case "delete":
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			log.Fatal(`uso: pg delete <app> <tabla> (--pk '<json>' | --filter col:op:val...) [--yes]`)
		}
		var pkJSON string
		var rawFilters []string
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--pk":
				i++
				if i >= len(args) {
					log.Fatal(`uso: pg delete <app> <tabla> --pk '<json>'`)
				}
				pkJSON = args[i]
			case "--filter":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg delete <app> <tabla> --filter col:op:val")
				}
				rawFilters = append(rawFilters, args[i])
			case "--yes", "-y":
			default:
				log.Fatalf("pg delete: flag desconocido %q", args[i])
			}
		}
		yes := hasYes(args[3:])
		var pk map[string]any
		if pkJSON != "" {
			pk = mustJSONMap(pkJSON)
		}
		filters, err := pg.ParseFilters(rawFilters)
		if err != nil {
			log.Fatalf("pg delete: %v", err)
		}
		if !yes {
			if prompt(fmt.Sprintf("borrar en %s.%s (pk=%s filtros=%v)", args[1], args[2], pkJSON, rawFilters), "no") != "si" {
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
			log.Fatalf("pg delete: %v", err)
		}
		defer cdb.Close()
		n, err := pg.DeleteRows(ctx, cdb, rec.Schema, args[2], pk, filters)
		if err != nil {
			log.Fatalf("pg delete: %v", err)
		}
		fmt.Printf("ok, borradas %d\n", n)
	case "create-table":
		if len(args) < 4 || args[1] == "" || args[2] == "" || args[3] == "" {
			log.Fatal(`uso: pg create-table <app> <tabla> '[{"name":"id","type":"uuid","pk":true}]' [--yes]`)
		}
		yes := hasYes(args[4:])
		var cols []pg.ColumnDef
		dec := json.NewDecoder(strings.NewReader(args[3]))
		if err := dec.Decode(&cols); err != nil {
			log.Fatalf("pg create-table: columnas JSON inválidas: %v", err)
		}
		if !yes {
			if prompt(fmt.Sprintf("crear tabla %s.%s (%d columnas)", args[1], args[2], len(cols)), "no") != "si" {
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
			log.Fatalf("pg create-table: %v", err)
		}
		defer cdb.Close()
		if err := pg.CreateTableExec(ctx, cdb, rec.Schema, args[2], cols); err != nil {
			log.Fatalf("pg create-table: %v", err)
		}
		fmt.Printf("tabla %s creada\n", args[2])
	case "add-column":
		if len(args) < 5 || args[1] == "" || args[2] == "" || args[3] == "" || args[4] == "" {
			log.Fatal("uso: pg add-column <app> <tabla> <col> <tipo> [--not-null] [--default ...] [--yes]")
		}
		var col pg.ColumnDef
		col.Name, col.Type = args[3], args[4]
		rest := args[5:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--not-null":
				col.NotNull = true
			case "--default":
				i++
				if i >= len(rest) {
					log.Fatal("uso: pg add-column <app> <tabla> <col> <tipo> [--default ...]")
				}
				col.Default = rest[i]
			case "--yes", "-y":
			default:
				log.Fatalf("pg add-column: flag desconocido %q", rest[i])
			}
		}
		if !hasYes(rest) {
			if prompt(fmt.Sprintf("añadir columna %s %s a %s.%s", col.Name, col.Type, args[1], args[2]), "no") != "si" {
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
			log.Fatalf("pg add-column: %v", err)
		}
		defer cdb.Close()
		if err := pg.AddColumnExec(ctx, cdb, rec.Schema, args[2], col); err != nil {
			log.Fatalf("pg add-column: %v", err)
		}
		fmt.Printf("columna %s añadida\n", col.Name)
	case "drop-table":
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			log.Fatal("uso: pg drop-table <app> <tabla> [--yes]")
		}
		if !hasYes(args[3:]) {
			if prompt("borrar tabla "+args[1]+"."+args[2]+" (los datos se pierden)", "no") != "si" {
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
			log.Fatalf("pg drop-table: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropTableExec(ctx, cdb, rec.Schema, args[2]); err != nil {
			log.Fatalf("pg drop-table: %v", err)
		}
		fmt.Printf("tabla %s borrada\n", args[2])
	case "drop-column":
		if len(args) < 4 || args[1] == "" || args[2] == "" || args[3] == "" {
			log.Fatal("uso: pg drop-column <app> <tabla> <col> [--yes]")
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("borrar columna %s de %s.%s", args[3], args[1], args[2]), "no") != "si" {
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
			log.Fatalf("pg drop-column: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropColumnExec(ctx, cdb, rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg drop-column: %v", err)
		}
		fmt.Printf("columna %s borrada\n", args[3])
	case "indexes":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg indexes <app> [tabla]")
		}
		table := ""
		if len(args) == 3 {
			if args[2] == "" || strings.HasPrefix(args[2], "-") {
				log.Fatal("uso: pg indexes <app> [tabla]")
			}
			table = args[2]
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
			log.Fatalf("pg indexes: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		idxs, err := pg.ListIndexes(ctx2, cdb, rec.Schema, table)
		if err != nil {
			log.Fatalf("pg indexes: %v", err)
		}
		for _, ix := range idxs {
			fmt.Printf("%s %s unique=%v primary=%v :: %s\n", ix.Name, ix.Table, ix.Unique, ix.Primary, ix.Def)
		}
	case "create-index":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg create-index <app> <tabla> <col,...> [--name n] [--unique] [--method btree|hash|gin|gist|brin] [--yes]")
		}
		name, method := "", "btree"
		unique := false
		rest := args[4:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--name":
				i++
				if i >= len(rest) || rest[i] == "" || strings.HasPrefix(rest[i], "-") {
					log.Fatal("uso: pg create-index <app> <tabla> <col,...> [--name n]")
				}
				name = rest[i]
			case "--unique":
				unique = true
			case "--method":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg create-index <app> <tabla> <col,...> [--method btree|hash|gin|gist|brin]")
				}
				method = rest[i]
			case "--yes", "-y":
			default:
				log.Fatalf("pg create-index: flag desconocido %q", rest[i])
			}
		}
		var cols []string
		for _, c := range strings.Split(args[3], ",") {
			if t := strings.TrimSpace(c); t != "" {
				cols = append(cols, t)
			}
		}
		if len(cols) == 0 {
			log.Fatal("uso: pg create-index <app> <tabla> <col,...> [--name n] [--unique] [--method btree|hash|gin|gist|brin] [--yes]")
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildCreateIndex(rec.Schema, args[2], name, cols, unique, method); err != nil {
			log.Fatalf("pg create-index: %v", err)
		}
		if !hasYes(rest) {
			if prompt(fmt.Sprintf("crear índice en %s.%s (%s)", args[1], args[2], strings.Join(cols, ", ")), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg create-index: %v", err)
		}
		defer cdb.Close()
		if err := pg.CreateIndexExec(ctx, cdb, rec.Schema, args[2], name, cols, unique, method); err != nil {
			log.Fatalf("pg create-index: %v", err)
		}
		fmt.Printf("índice creado en %s\n", args[2])
	case "drop-index":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg drop-index <app> <index> [--yes]")
		}
		for _, a := range args[3:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg drop-index: flag desconocido %q (uso: pg drop-index <app> <index> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildDropIndex(rec.Schema, args[2]); err != nil {
			log.Fatalf("pg drop-index: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt("borrar índice "+args[1]+"."+args[2]+" (la tabla conserva los datos)", "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg drop-index: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropIndexExec(ctx, cdb, rec.Schema, args[2]); err != nil {
			log.Fatalf("pg drop-index: %v", err)
		}
		fmt.Printf("índice %s borrado\n", args[2])
	case "constraints":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg constraints <app> [tabla]")
		}
		table := ""
		if len(args) == 3 {
			if args[2] == "" || strings.HasPrefix(args[2], "-") {
				log.Fatal("uso: pg constraints <app> [tabla]")
			}
			table = args[2]
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
			log.Fatalf("pg constraints: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		cons, err := pg.ListConstraints(ctx2, cdb, rec.Schema, table)
		if err != nil {
			log.Fatalf("pg constraints: %v", err)
		}
		for _, c := range cons {
			fmt.Printf("%s.%s [%s] :: %s\n", c.Table, c.Name, c.Kind, c.Def)
		}
	case "policies":
		if len(args) != 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg policies <app> <tabla>")
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
			log.Fatalf("pg policies: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		pols, err := pg.ListPolicies(ctx2, cdb, rec.Schema, args[2])
		if err != nil {
			log.Fatalf("pg policies: %v", err)
		}
		for _, p := range pols {
			fmt.Printf("policy %s %s roles=%s\n", p.Name, p.Command, p.Roles)
		}
	case "create-policy":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg create-policy <app> <tabla> <nombre> [--cmd select|insert|update|delete|all] [--roles a,b] [--using expr] [--check expr] [--yes]")
		}
		cmd, rolesCSV, using, check := "all", "", "", ""
		rest := args[4:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--cmd":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg create-policy <app> <tabla> <nombre> [--cmd select|insert|update|delete|all]")
				}
				cmd = rest[i]
			case "--roles":
				i++
				if i >= len(rest) {
					log.Fatal("uso: pg create-policy <app> <tabla> <nombre> [--roles a,b]")
				}
				rolesCSV = rest[i]
			case "--using":
				i++
				if i >= len(rest) {
					log.Fatal("uso: pg create-policy <app> <tabla> <nombre> [--using expr]")
				}
				using = rest[i]
			case "--check":
				i++
				if i >= len(rest) {
					log.Fatal("uso: pg create-policy <app> <tabla> <nombre> [--check expr]")
				}
				check = rest[i]
			case "--yes", "-y":
			default:
				log.Fatalf("pg create-policy: flag desconocido %q", rest[i])
			}
		}
		roles := pg.SplitRoles(rolesCSV)
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildCreatePolicy(rec.Schema, args[2], args[3], cmd, roles, using, check); err != nil {
			log.Fatalf("pg create-policy: %v", err)
		}
		if !hasYes(rest) {
			if prompt(fmt.Sprintf("crear policy %s en %s.%s", args[3], args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg create-policy: %v", err)
		}
		defer cdb.Close()
		if err := pg.CreatePolicyExec(ctx, cdb, rec.Schema, args[2], args[3], cmd, roles, using, check); err != nil {
			log.Fatalf("pg create-policy: %v", err)
		}
		fmt.Printf("policy %s creada\n", args[3])
	case "drop-policy":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg drop-policy <app> <tabla> <nombre> [--yes]")
		}
		for _, a := range args[4:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg drop-policy: flag desconocido %q (uso: pg drop-policy <app> <tabla> <nombre> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildDropPolicy(rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg drop-policy: %v", err)
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("borrar policy %s de %s.%s", args[3], args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg drop-policy: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropPolicyExec(ctx, cdb, rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg drop-policy: %v", err)
		}
		fmt.Printf("policy %s borrada\n", args[3])
	case "rename-table":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg rename-table <app> <tabla> <nueva> [--yes]")
		}
		for _, a := range args[4:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg rename-table: flag desconocido %q (uso: pg rename-table <app> <tabla> <nueva> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildRenameTable(rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg rename-table: %v", err)
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("renombrar tabla %s.%s a %s", args[1], args[2], args[3]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg rename-table: %v", err)
		}
		defer cdb.Close()
		if err := pg.RenameTableExec(ctx, cdb, rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg rename-table: %v", err)
		}
		fmt.Printf("tabla %s renombrada a %s\n", args[2], args[3])
	case "rename-column":
		if len(args) < 5 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") || args[4] == "" || strings.HasPrefix(args[4], "-") {
			log.Fatal("uso: pg rename-column <app> <tabla> <col> <nueva> [--yes]")
		}
		for _, a := range args[5:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg rename-column: flag desconocido %q (uso: pg rename-column <app> <tabla> <col> <nueva> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildRenameColumn(rec.Schema, args[2], args[3], args[4]); err != nil {
			log.Fatalf("pg rename-column: %v", err)
		}
		if !hasYes(args[5:]) {
			if prompt(fmt.Sprintf("renombrar columna %s de %s.%s a %s", args[3], args[1], args[2], args[4]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg rename-column: %v", err)
		}
		defer cdb.Close()
		if err := pg.RenameColumnExec(ctx, cdb, rec.Schema, args[2], args[3], args[4]); err != nil {
			log.Fatalf("pg rename-column: %v", err)
		}
		fmt.Printf("columna %s renombrada a %s\n", args[3], args[4])
	case "alter-column":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg alter-column <app> <tabla> <col> [--type T] [--default D] [--drop-default] [--not-null|--drop-not-null] [--yes]")
		}
		var typ, def string
		hasType, hasDef, dropDef, notNull, dropNotNull := false, false, false, false, false
		rest := args[4:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--type":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg alter-column <app> <tabla> <col> [--type T]")
				}
				typ, hasType = rest[i], true
			case "--default":
				i++
				if i >= len(rest) {
					log.Fatal("uso: pg alter-column <app> <tabla> <col> [--default D]")
				}
				def, hasDef = rest[i], true
			case "--drop-default":
				dropDef = true
			case "--not-null":
				notNull = true
			case "--drop-not-null":
				dropNotNull = true
			case "--yes", "-y":
			default:
				log.Fatalf("pg alter-column: flag desconocido %q", rest[i])
			}
		}
		if hasDef && dropDef {
			log.Fatal("pg alter-column: --default y --drop-default excluyentes")
		}
		if notNull && dropNotNull {
			log.Fatal("pg alter-column: --not-null y --drop-not-null excluyentes")
		}
		if !hasType && !hasDef && !dropDef && !notNull && !dropNotNull {
			log.Fatal("uso: pg alter-column <app> <tabla> <col> [--type T] [--default D] [--drop-default] [--not-null|--drop-not-null] [--yes] (al menos un cambio)")
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if hasType {
			if _, err := pg.BuildAlterColumnType(rec.Schema, args[2], args[3], typ); err != nil {
				log.Fatalf("pg alter-column: %v", err)
			}
		}
		if hasDef || dropDef {
			d := def
			if dropDef {
				d = ""
			}
			if _, err := pg.BuildAlterColumnDefault(rec.Schema, args[2], args[3], d, dropDef); err != nil {
				log.Fatalf("pg alter-column: %v", err)
			}
		}
		if notNull || dropNotNull {
			if _, err := pg.BuildAlterColumnNullable(rec.Schema, args[2], args[3], notNull); err != nil {
				log.Fatalf("pg alter-column: %v", err)
			}
		}
		if !hasYes(rest) {
			if prompt(fmt.Sprintf("modificar columna %s de %s.%s", args[3], args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg alter-column: %v", err)
		}
		defer cdb.Close()
		if hasType {
			if err := pg.AlterColumnTypeExec(ctx, cdb, rec.Schema, args[2], args[3], typ); err != nil {
				log.Fatalf("pg alter-column: %v", err)
			}
		}
		if hasDef || dropDef {
			d := def
			if dropDef {
				d = ""
			}
			if err := pg.AlterColumnDefaultExec(ctx, cdb, rec.Schema, args[2], args[3], d, dropDef); err != nil {
				log.Fatalf("pg alter-column: %v", err)
			}
		}
		if notNull || dropNotNull {
			if err := pg.AlterColumnNullableExec(ctx, cdb, rec.Schema, args[2], args[3], notNull); err != nil {
				log.Fatalf("pg alter-column: %v", err)
			}
		}
		fmt.Printf("columna %s modificada\n", args[3])
	case "truncate":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg truncate <app> <tabla> [--yes]")
		}
		for _, a := range args[3:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg truncate: flag desconocido %q (uso: pg truncate <app> <tabla> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildTruncate(rec.Schema, args[2]); err != nil {
			log.Fatalf("pg truncate: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt("vaciar tabla "+args[1]+"."+args[2]+" (los datos se pierden)", "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg truncate: %v", err)
		}
		defer cdb.Close()
		if err := pg.TruncateExec(ctx, cdb, rec.Schema, args[2]); err != nil {
			log.Fatalf("pg truncate: %v", err)
		}
		fmt.Printf("tabla %s vaciada\n", args[2])
	case "duplicate":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg duplicate <app> <tabla> <nueva> [--with-data] [--yes]")
		}
		withData := false
		for _, a := range args[4:] {
			switch a {
			case "--with-data":
				withData = true
			case "--yes", "-y":
			default:
				log.Fatalf("pg duplicate: flag desconocido %q (uso: pg duplicate <app> <tabla> <nueva> [--with-data] [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildDuplicate(rec.Schema, args[2], args[3], withData); err != nil {
			log.Fatalf("pg duplicate: %v", err)
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("duplicar tabla %s.%s a %s", args[1], args[2], args[3]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg duplicate: %v", err)
		}
		defer cdb.Close()
		if err := pg.DuplicateExec(ctx, cdb, rec.Schema, args[2], args[3], withData); err != nil {
			log.Fatalf("pg duplicate: %v", err)
		}
		fmt.Printf("tabla %s duplicada a %s\n", args[2], args[3])
	case "backups":
		if len(args) != 2 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg backups <app>")
		}
		dir := filepath.Join(logx.DataDir(cfgPath), "backups")
		bs, err := pg.ListBackups(dir, args[1])
		if err != nil {
			log.Fatalf("pg backups: %v", err)
		}
		if len(bs) == 0 {
			fmt.Println("(sin backups)")
			break
		}
		for _, b := range bs {
			fmt.Printf("%s %d %s\n", b.Name, b.Size, b.Mtime)
		}
	case "backup-delete":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg backup-delete <app> <nombre> [--yes]")
		}
		for _, a := range args[3:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg backup-delete: flag desconocido %q (uso: pg backup-delete <app> <nombre> [--yes])", a)
			}
		}
		if !hasYes(args[3:]) {
			if prompt(fmt.Sprintf("borrar backup %s de %s (no se puede deshacer)", args[2], args[1]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		dir := filepath.Join(logx.DataDir(cfgPath), "backups")
		if err := pg.DeleteBackup(dir, args[1], args[2]); err != nil {
			log.Fatalf("pg backup-delete: %v", err)
		}
		fmt.Printf("backup %s borrado\n", args[2])
	case "rows-export":
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			log.Fatal("uso: pg rows-export <app> <tabla> [--filter col:op:val]... [--search q] [--order col] [--dir asc|desc] [--out archivo.csv]")
		}
		var order, dir, search, outPath string
		var rawFilters []string
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--filter":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows-export <app> <tabla> [--filter col:op:val]")
				}
				rawFilters = append(rawFilters, args[i])
			case "--search":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows-export <app> <tabla> [--search q]")
				}
				search = args[i]
			case "--order":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows-export <app> <tabla> [--order col]")
				}
				order = args[i]
			case "--dir":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows-export <app> <tabla> [--dir asc|desc]")
				}
				dir = args[i]
			case "--out":
				i++
				if i >= len(args) {
					log.Fatal("uso: pg rows-export <app> <tabla> [--out archivo.csv]")
				}
				outPath = args[i]
			default:
				log.Fatalf("pg rows-export: flag desconocido %q", args[i])
			}
		}
		filters, err := pg.ParseFilters(rawFilters)
		if err != nil {
			log.Fatalf("pg rows-export: %v", err)
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
			log.Fatalf("pg rows-export: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		var searchCols []string
		if strings.TrimSpace(search) != "" {
			cis, err := pg.ColumnTypes(ctx2, cdb, rec.Schema, args[2])
			if err != nil {
				log.Fatalf("pg rows-export: %v", err)
			}
			for _, c := range cis {
				t := strings.ToLower(c.Type)
				if strings.Contains(t, "char") || strings.Contains(t, "text") || t == "uuid" || t == "json" || t == "jsonb" || t == "inet" {
					searchCols = append(searchCols, c.Name)
				}
				if len(searchCols) >= 10 {
					break
				}
			}
		}
		q, qargs, err := pg.ExportQuery(rec.Schema, args[2], pg.RowsQuery{Filters: filters, Search: search, OrderCol: order, OrderDir: dir}, searchCols)
		if err != nil {
			log.Fatalf("pg rows-export: %v", err)
		}
		rs, err := cdb.QueryContext(ctx2, q, qargs...)
		if err != nil {
			log.Fatalf("pg rows-export: %v", err)
		}
		cols, err := rs.Columns()
		if err != nil {
			rs.Close()
			log.Fatalf("pg rows-export: %v", err)
		}
		var data [][]string
		for rs.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rs.Scan(ptrs...); err != nil {
				rs.Close()
				log.Fatalf("pg rows-export: %v", err)
			}
			line := make([]string, len(cols))
			for i, v := range vals {
				switch t := v.(type) {
				case nil:
					line[i] = ""
				case []byte:
					line[i] = string(t)
				case time.Time:
					line[i] = t.UTC().Format(time.RFC3339)
				default:
					line[i] = fmt.Sprint(v)
				}
			}
			data = append(data, line)
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			log.Fatalf("pg rows-export: %v", err)
		}
		rs.Close()
		if outPath != "" {
			f, err := os.Create(outPath)
			if err != nil {
				log.Fatalf("pg rows-export: %v", err)
			}
			w := csv.NewWriter(f)
			if err := w.Write(cols); err != nil {
				f.Close()
				log.Fatalf("pg rows-export: %v", err)
			}
			for _, line := range data {
				if err := w.Write(line); err != nil {
					f.Close()
					log.Fatalf("pg rows-export: %v", err)
				}
			}
			w.Flush()
			if err := w.Error(); err != nil {
				f.Close()
				log.Fatalf("pg rows-export: %v", err)
			}
			if err := f.Close(); err != nil {
				log.Fatalf("pg rows-export: %v", err)
			}
			fmt.Printf("exportadas %d filas a %s\n", len(data), outPath)
			break
		}
		w := csv.NewWriter(os.Stdout)
		if err := w.Write(cols); err != nil {
			log.Fatalf("pg rows-export: %v", err)
		}
		for _, line := range data {
			if err := w.Write(line); err != nil {
				log.Fatalf("pg rows-export: %v", err)
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			log.Fatalf("pg rows-export: %v", err)
		}
	case "rows-import":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg rows-import <app> <tabla> <archivo.csv> [--yes]")
		}
		for _, a := range args[4:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg rows-import: flag desconocido %q (uso: pg rows-import <app> <tabla> <archivo.csv> [--yes])", a)
			}
		}
		f, err := os.Open(args[3])
		if err != nil {
			log.Fatalf("pg rows-import: %v", err)
		}
		r := csv.NewReader(f)
		records, err := r.ReadAll()
		f.Close()
		if err != nil {
			log.Fatalf("pg rows-import: csv inválido: %v", err)
		}
		if len(records) == 0 {
			log.Fatal("pg rows-import: csv vacío (primera fila = columnas)")
		}
		columns := make([]string, 0, len(records[0]))
		for _, c := range records[0] {
			if t := strings.TrimSpace(c); t == "" {
				log.Fatal("pg rows-import: columna vacía en cabecera")
			} else {
				columns = append(columns, t)
			}
		}
		if len(records)-1 == 0 {
			log.Fatal("pg rows-import: sin filas para importar")
		}
		if len(records)-1 > 1000 {
			log.Fatal("pg rows-import: máx 1000 filas por lote")
		}
		rowsAny := make([][]any, 0, len(records)-1)
		for _, line := range records[1:] {
			row := make([]any, len(line))
			for i, v := range line {
				row[i] = v
			}
			rowsAny = append(rowsAny, row)
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, _, err := pg.BuildInsertMany(rec.Schema, args[2], columns, rowsAny); err != nil {
			log.Fatalf("pg rows-import: %v", err)
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("importar %d filas en %s.%s desde %s", len(rowsAny), args[1], args[2], args[3]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg rows-import: %v", err)
		}
		defer cdb.Close()
		n, err := pg.InsertManyExec(ctx, cdb, rec.Schema, args[2], columns, rowsAny)
		if err != nil {
			log.Fatalf("pg rows-import: %v", err)
		}
		fmt.Printf("ok, importadas %d\n", n)
	case "explain":
		if len(args) < 3 || args[1] == "" {
			log.Fatal("uso: pg explain <app> <sql> [--yes]")
		}
		yes := false
		var sqlParts []string
		for _, a := range args[2:] {
			if a == "--yes" || a == "-y" {
				yes = true
				continue
			}
			sqlParts = append(sqlParts, a)
		}
		sqlText := strings.Join(sqlParts, " ")
		if strings.TrimSpace(sqlText) == "" {
			log.Fatal("uso: pg explain <app> <sql> [--yes]")
		}
		if !yes {
			if prompt("explicar en "+args[1]+": "+sqlText, "no") != "si" {
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
			log.Fatalf("pg explain: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		rows, cols, _, err := pg.Explain(ctx2, cdb, sqlText)
		if err != nil {
			log.Fatalf("pg explain: %v", err)
		}
		fmt.Println(strings.Join(cols, "\t"))
		for _, r := range rows {
			cells := make([]string, len(r))
			for i, v := range r {
				cells[i] = fmt.Sprint(v)
			}
			fmt.Println(strings.Join(cells, "\t"))
		}
	case "views":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg views <app> [nombre]")
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
			log.Fatalf("pg views: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		if len(args) == 3 {
			if args[2] == "" || strings.HasPrefix(args[2], "-") {
				log.Fatal("uso: pg views <app> [nombre]")
			}
			v, err := pg.GetViewDef(ctx2, cdb, rec.Schema, args[2])
			if err != nil {
				log.Fatalf("pg views: %v", err)
			}
			fmt.Printf("%s %s\n%s\n", v.Name, v.Kind, v.Def)
			break
		}
		views, err := pg.ListViews(ctx2, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg views: %v", err)
		}
		matviews, err := pg.ListMatviews(ctx2, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg views: %v", err)
		}
		for _, v := range views {
			fmt.Printf("%s %s\n", v.Name, v.Kind)
		}
		for _, v := range matviews {
			fmt.Printf("%s %s\n", v.Name, v.Kind)
		}
	case "create-view":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg create-view <app> <nombre> <sql...> [--materialized] [--or-replace] [--with-data] [--yes]")
		}
		materialized, orReplace, withData := false, false, false
		var sqlParts []string
		for _, a := range args[3:] {
			switch a {
			case "--materialized":
				materialized = true
			case "--or-replace":
				orReplace = true
			case "--with-data":
				withData = true
			case "--yes", "-y":
			default:
				if strings.HasPrefix(a, "--") {
					log.Fatalf("pg create-view: flag desconocido %q", a)
				}
				sqlParts = append(sqlParts, a)
			}
		}
		selectSQL := strings.Join(sqlParts, " ")
		if strings.TrimSpace(selectSQL) == "" {
			log.Fatal("uso: pg create-view <app> <nombre> <sql...> [--materialized] [--or-replace] [--with-data] [--yes]")
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildCreateView(rec.Schema, args[2], selectSQL, materialized, orReplace, withData); err != nil {
			log.Fatalf("pg create-view: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt(fmt.Sprintf("crear vista %s.%s", args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg create-view: %v", err)
		}
		defer cdb.Close()
		if err := pg.CreateViewExec(ctx, cdb, rec.Schema, args[2], selectSQL, materialized, orReplace, withData); err != nil {
			log.Fatalf("pg create-view: %v", err)
		}
		fmt.Printf("vista %s creada\n", args[2])
	case "drop-view":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg drop-view <app> <nombre> [--yes]")
		}
		for _, a := range args[3:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg drop-view: flag desconocido %q (uso: pg drop-view <app> <nombre> [--yes])", a)
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
			log.Fatalf("pg drop-view: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		v, err := pg.GetViewDef(ctx2, cdb, rec.Schema, args[2])
		if err != nil {
			log.Fatalf("pg drop-view: %v", err)
		}
		materialized := v.Kind == "materialized"
		if _, err := pg.BuildDropView(rec.Schema, args[2], materialized); err != nil {
			log.Fatalf("pg drop-view: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt(fmt.Sprintf("borrar vista %s.%s", args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		if err := pg.DropViewExec(ctx, cdb, rec.Schema, args[2], materialized); err != nil {
			log.Fatalf("pg drop-view: %v", err)
		}
		fmt.Printf("vista %s borrada\n", args[2])
	case "matview-refresh":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg matview-refresh <app> <nombre> [--no-data] [--yes]")
		}
		withData := true
		for _, a := range args[3:] {
			switch a {
			case "--no-data":
				withData = false
			case "--yes", "-y":
			default:
				log.Fatalf("pg matview-refresh: flag desconocido %q (uso: pg matview-refresh <app> <nombre> [--no-data] [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildRefreshMatview(rec.Schema, args[2], withData); err != nil {
			log.Fatalf("pg matview-refresh: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt(fmt.Sprintf("refrescar vista materializada %s.%s", args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg matview-refresh: %v", err)
		}
		defer cdb.Close()
		if err := pg.RefreshMatviewExec(ctx, cdb, rec.Schema, args[2], withData); err != nil {
			log.Fatalf("pg matview-refresh: %v", err)
		}
		fmt.Printf("vista materializada %s refrescada\n", args[2])
	case "functions":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg functions <app> [identidad]")
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
			log.Fatalf("pg functions: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		if len(args) == 3 {
			if args[2] == "" || strings.HasPrefix(args[2], "-") {
				log.Fatal("uso: pg functions <app> [identidad]")
			}
			f, err := pg.GetRoutineDef(ctx2, cdb, rec.Schema, args[2])
			if err != nil {
				log.Fatalf("pg functions: %v", err)
			}
			fmt.Println(f.Def)
			break
		}
		fs, err := pg.ListFunctions(ctx2, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg functions: %v", err)
		}
		for _, f := range fs {
			fmt.Printf("%s\t%s\t%s\n", f.Identity, f.Return, f.Lang)
		}
	case "drop-function":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg drop-function <app> <identidad> [--yes]")
		}
		for _, a := range args[3:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg drop-function: flag desconocido %q (uso: pg drop-function <app> <identidad> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildDropRoutine(rec.Schema, args[2], false); err != nil {
			log.Fatalf("pg drop-function: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt(fmt.Sprintf("borrar función %s de %s", args[2], args[1]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg drop-function: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropRoutineExec(ctx, cdb, rec.Schema, args[2], false); err != nil {
			log.Fatalf("pg drop-function: %v", err)
		}
		fmt.Printf("función %s borrada\n", args[2])
	case "procedures":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg procedures <app> [identidad]")
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
			log.Fatalf("pg procedures: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		if len(args) == 3 {
			if args[2] == "" || strings.HasPrefix(args[2], "-") {
				log.Fatal("uso: pg procedures <app> [identidad]")
			}
			p, err := pg.GetRoutineDef(ctx2, cdb, rec.Schema, args[2])
			if err != nil {
				log.Fatalf("pg procedures: %v", err)
			}
			fmt.Println(p.Def)
			break
		}
		ps, err := pg.ListProcedures(ctx2, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg procedures: %v", err)
		}
		for _, p := range ps {
			fmt.Printf("%s\t%s\t%s\n", p.Identity, p.Return, p.Lang)
		}
	case "proc-call":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg proc-call <app> <call> [--yes]")
		}
		var callParts []string
		for _, a := range args[2:] {
			if a == "--yes" || a == "-y" {
				continue
			}
			if strings.HasPrefix(a, "--") {
				log.Fatalf("pg proc-call: flag desconocido %q (uso: pg proc-call <app> <call> [--yes])", a)
			}
			callParts = append(callParts, a)
		}
		call := strings.Join(callParts, " ")
		if strings.TrimSpace(call) == "" {
			log.Fatal("uso: pg proc-call <app> <call> [--yes]")
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildCallProcedure(rec.Schema, call); err != nil {
			log.Fatalf("pg proc-call: %v", err)
		}
		if !hasYes(args[2:]) {
			if prompt(fmt.Sprintf("llamar procedimiento %s en %s", call, args[1]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg proc-call: %v", err)
		}
		defer cdb.Close()
		res, err := pg.CallProcedureExec(ctx, cdb, rec.Schema, call)
		if err != nil {
			log.Fatalf("pg proc-call: %v", err)
		}
		fmt.Printf("ok, afectadas %d\n", res.Affected)
	case "drop-procedure":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg drop-procedure <app> <identidad> [--yes]")
		}
		for _, a := range args[3:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg drop-procedure: flag desconocido %q (uso: pg drop-procedure <app> <identidad> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildDropRoutine(rec.Schema, args[2], true); err != nil {
			log.Fatalf("pg drop-procedure: %v", err)
		}
		if !hasYes(args[3:]) {
			if prompt(fmt.Sprintf("borrar procedimiento %s de %s", args[2], args[1]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg drop-procedure: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropRoutineExec(ctx, cdb, rec.Schema, args[2], true); err != nil {
			log.Fatalf("pg drop-procedure: %v", err)
		}
		fmt.Printf("procedimiento %s borrado\n", args[2])
	case "triggers":
		if len(args) < 2 || len(args) > 3 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg triggers <app> [tabla]")
		}
		table := ""
		if len(args) == 3 {
			if args[2] == "" || strings.HasPrefix(args[2], "-") {
				log.Fatal("uso: pg triggers <app> [tabla]")
			}
			table = args[2]
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
			log.Fatalf("pg triggers: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		trs, err := pg.ListTriggers(ctx2, cdb, rec.Schema, table)
		if err != nil {
			log.Fatalf("pg triggers: %v", err)
		}
		for _, tr := range trs {
			fmt.Printf("%s.%s %s %s %s :: %s\n", tr.Table, tr.Name, tr.Timing, tr.Events, tr.Orientation, tr.Function)
		}
	case "create-trigger":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> --timing before|after|instead --events insert,update --function f() [--orientation row|statement] [--when cond] [--yes]")
		}
		var timing, eventsCSV, function, orientation, when string
		rest := args[4:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--timing":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> --timing before|after|instead")
				}
				timing = rest[i]
			case "--events":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> --events insert,update")
				}
				eventsCSV = rest[i]
			case "--function":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> --function f()")
				}
				function = rest[i]
			case "--orientation":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> [--orientation row|statement]")
				}
				orientation = rest[i]
			case "--when":
				i++
				if i >= len(rest) {
					log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> [--when cond]")
				}
				when = rest[i]
			case "--yes", "-y":
			default:
				log.Fatalf("pg create-trigger: flag desconocido %q", rest[i])
			}
		}
		if timing == "" || eventsCSV == "" || function == "" {
			log.Fatal("uso: pg create-trigger <app> <tabla> <nombre> --timing before|after|instead --events insert,update --function f() [--orientation row|statement] [--when cond] [--yes]")
		}
		if strings.ToLower(strings.TrimSpace(timing)) == "instead" {
			timing = "instead of"
		}
		var events []string
		for _, e := range strings.Split(eventsCSV, ",") {
			if t := strings.TrimSpace(e); t != "" {
				events = append(events, t)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildCreateTrigger(rec.Schema, args[2], args[3], timing, events, orientation, function, when); err != nil {
			log.Fatalf("pg create-trigger: %v", err)
		}
		if !hasYes(rest) {
			if prompt(fmt.Sprintf("crear trigger %s en %s.%s", args[3], args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg create-trigger: %v", err)
		}
		defer cdb.Close()
		if err := pg.CreateTriggerExec(ctx, cdb, rec.Schema, args[2], args[3], timing, events, orientation, function, when); err != nil {
			log.Fatalf("pg create-trigger: %v", err)
		}
		fmt.Printf("trigger %s creado\n", args[3])
	case "drop-trigger":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg drop-trigger <app> <tabla> <nombre> [--yes]")
		}
		for _, a := range args[4:] {
			if a != "--yes" && a != "-y" {
				log.Fatalf("pg drop-trigger: flag desconocido %q (uso: pg drop-trigger <app> <tabla> <nombre> [--yes])", a)
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildDropTrigger(rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg drop-trigger: %v", err)
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("borrar trigger %s de %s.%s", args[3], args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg drop-trigger: %v", err)
		}
		defer cdb.Close()
		if err := pg.DropTriggerExec(ctx, cdb, rec.Schema, args[2], args[3]); err != nil {
			log.Fatalf("pg drop-trigger: %v", err)
		}
		fmt.Printf("trigger %s borrado\n", args[3])
	case "trigger-toggle":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" || strings.HasPrefix(args[3], "-") {
			log.Fatal("uso: pg trigger-toggle <app> <tabla> <nombre> --enable|--disable [--yes]")
		}
		enable, disable := false, false
		for _, a := range args[4:] {
			switch a {
			case "--enable":
				enable = true
			case "--disable":
				disable = true
			case "--yes", "-y":
			default:
				log.Fatalf("pg trigger-toggle: flag desconocido %q (uso: pg trigger-toggle <app> <tabla> <nombre> --enable|--disable [--yes])", a)
			}
		}
		if enable == disable {
			log.Fatal("uso: pg trigger-toggle <app> <tabla> <nombre> --enable|--disable [--yes]")
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildToggleTrigger(rec.Schema, args[2], args[3], enable); err != nil {
			log.Fatalf("pg trigger-toggle: %v", err)
		}
		if !hasYes(args[4:]) {
			verb := "desactivar"
			if enable {
				verb = "activar"
			}
			if prompt(fmt.Sprintf("%s trigger %s de %s.%s", verb, args[3], args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg trigger-toggle: %v", err)
		}
		defer cdb.Close()
		if err := pg.ToggleTriggerExec(ctx, cdb, rec.Schema, args[2], args[3], enable); err != nil {
			log.Fatalf("pg trigger-toggle: %v", err)
		}
		state := "desactivado"
		if enable {
			state = "activado"
		}
		fmt.Printf("trigger %s %s\n", args[3], state)
	case "sequences":
		if len(args) != 2 || args[1] == "" || strings.HasPrefix(args[1], "-") {
			log.Fatal("uso: pg sequences <app>")
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
			log.Fatalf("pg sequences: %v", err)
		}
		defer cdb.Close()
		ctx2, cancel2 := pgCtx()
		defer cancel2()
		seqs, err := pg.ListSequences(ctx2, cdb, rec.Schema)
		if err != nil {
			log.Fatalf("pg sequences: %v", err)
		}
		for _, s := range seqs {
			last := "-"
			if s.LastValue != nil {
				last = fmt.Sprint(*s.LastValue)
			}
			fmt.Printf("%s %s last=%s start=%d\n", s.Name, s.DataType, last, s.Start)
		}
	case "seq-restart":
		if len(args) < 3 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") {
			log.Fatal("uso: pg seq-restart <app> <nombre> [--with n] [--yes]")
		}
		var with *int64
		rest := args[3:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--with":
				i++
				if i >= len(rest) || rest[i] == "" {
					log.Fatal("uso: pg seq-restart <app> <nombre> [--with n]")
				}
				n, err := strconv.ParseInt(rest[i], 10, 64)
				if err != nil {
					log.Fatalf("pg seq-restart: --with inválido: %v", err)
				}
				v := n
				with = &v
			case "--yes", "-y":
			default:
				log.Fatalf("pg seq-restart: flag desconocido %q (uso: pg seq-restart <app> <nombre> [--with n] [--yes])", rest[i])
			}
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildRestartSequence(rec.Schema, args[2], with); err != nil {
			log.Fatalf("pg seq-restart: %v", err)
		}
		if !hasYes(rest) {
			if prompt(fmt.Sprintf("reiniciar secuencia %s.%s", args[1], args[2]), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg seq-restart: %v", err)
		}
		defer cdb.Close()
		if err := pg.RestartSequenceExec(ctx, cdb, rec.Schema, args[2], with); err != nil {
			log.Fatalf("pg seq-restart: %v", err)
		}
		fmt.Printf("secuencia %s reiniciada\n", args[2])
	case "seq-setval":
		if len(args) < 4 || args[1] == "" || strings.HasPrefix(args[1], "-") || args[2] == "" || strings.HasPrefix(args[2], "-") || args[3] == "" {
			log.Fatal("uso: pg seq-setval <app> <nombre> <valor> [--no-called] [--yes]")
		}
		called := true
		for _, a := range args[4:] {
			switch a {
			case "--no-called":
				called = false
			case "--yes", "-y":
			default:
				log.Fatalf("pg seq-setval: flag desconocido %q (uso: pg seq-setval <app> <nombre> <valor> [--no-called] [--yes])", a)
			}
		}
		value, err := strconv.ParseInt(args[3], 10, 64)
		if err != nil {
			log.Fatalf("pg seq-setval: valor inválido: %v", err)
		}
		cfg := pgCfg(db)
		rec, ok, err := reg.PgGet(args[1])
		if err != nil || !ok {
			log.Fatalf("pg: %q sin base", args[1])
		}
		if _, err := pg.BuildSetval(rec.Schema, args[2], value, called); err != nil {
			log.Fatalf("pg seq-setval: %v", err)
		}
		if !hasYes(args[4:]) {
			if prompt(fmt.Sprintf("fijar secuencia %s.%s a %d", args[1], args[2], value), "no") != "si" {
				fmt.Println("cancelado")
				return
			}
		}
		ctx, cancel := pgCtx()
		defer cancel()
		cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
		if err != nil {
			log.Fatalf("pg seq-setval: %v", err)
		}
		defer cdb.Close()
		nv, err := pg.SetvalExec(ctx, cdb, rec.Schema, args[2], value, called)
		if err != nil {
			log.Fatalf("pg seq-setval: %v", err)
		}
		fmt.Printf("ok, nuevo valor %d\n", nv)
	default:
		log.Fatal("uso: pg <status|list|provision|drop|backup|restore|tables|rows|schema|insert|update|delete|create-table|add-column|drop-table|drop-column|query|indexes|create-index|drop-index|constraints|policies|create-policy|drop-policy|rename-table|rename-column|alter-column|truncate|duplicate|backups|backup-delete|rows-export|rows-import|explain|views|create-view|drop-view|matview-refresh|functions|drop-function|procedures|proc-call|drop-procedure|triggers|create-trigger|drop-trigger|trigger-toggle|sequences|seq-restart|seq-setval>")
	}
}

// printSchema vuelca el inspector de una tabla (columnas con tipo/nulo/default/PK).
func printSchema(s pg.TableSchema) {
	fmt.Printf("tabla %s (%d filas, RLS %s):\n", s.Table, s.Rows, map[bool]string{true: "on", false: "off"}[s.RLS])
	for _, c := range s.Columns {
		marks := ""
		if c.IsPK {
			marks += " PK"
		}
		if !c.Nullable {
			marks += " NOT NULL"
		}
		def := ""
		if c.Default != nil {
			def = " DEFAULT " + *c.Default
		}
		fmt.Printf("  %-20s %-22s%s%s\n", c.Name, c.Type, def, marks)
	}
}

// mustJSONMap parsea un objeto JSON con números enteros preservados.
func mustJSONMap(s string) map[string]any {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		log.Fatalf("JSON inválido: %v", err)
	}
	if m == nil {
		log.Fatal("JSON inválido: objeto vacío")
	}
	for k, v := range m {
		if n, ok := v.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				m[k] = i
			} else if f, err := n.Float64(); err == nil {
				m[k] = f
			} else {
				m[k] = n.String()
			}
		}
	}
	return m
}

func hasYes(args []string) bool {
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			return true
		}
	}
	return false
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
