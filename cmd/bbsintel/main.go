package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

type server struct { db *sql.DB }

func main() {
	listen := env("BBSINTEL_LISTEN", ":8080")
	dbPath := env("BBSINTEL_DB", "./data/bbsintel.db")
	s, err := store.Open(dbPath)
	if err != nil { log.Fatal(err) }
	defer s.Close()

	srv := &server{db:s.DB}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, map[string]string{"status":"ok"}) })
	mux.HandleFunc("GET /api/v1/bbs", srv.listBBS)
	mux.HandleFunc("GET /api/v1/bbs/{id}", srv.getBBS)
	mux.HandleFunc("GET /api/v1/stats", srv.stats)
	log.Printf("BBSIntel listening on %s", listen)
	log.Fatal(http.ListenAndServe(listen, mux))
}

func env(k, fallback string) string { if v:=os.Getenv(k); v!="" { return v }; return fallback }
func jsonOut(w http.ResponseWriter, v any) { w.Header().Set("Content-Type","application/json"); _=json.NewEncoder(w).Encode(v) }

func (s *server) listBBS(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,name,software,country FROM bbs ORDER BY lower(name)`)
	if err != nil { http.Error(w, err.Error(), 500); return }; defer rows.Close()
	out:=[]map[string]any{}
	for rows.Next(){ var id int64; var name,software,country string; if rows.Scan(&id,&name,&software,&country)==nil { out=append(out,map[string]any{"id":id,"name":name,"software":software,"country":country}) } }
	jsonOut(w,out)
}

func (s *server) getBBS(w http.ResponseWriter, r *http.Request) {
	id:=strings.TrimSpace(r.PathValue("id")); var name,software,country,description string
	err:=s.db.QueryRowContext(r.Context(),`SELECT name,software,country,description FROM bbs WHERE id=?`,id).Scan(&name,&software,&country,&description)
	if err==sql.ErrNoRows { http.NotFound(w,r); return }; if err!=nil { http.Error(w,err.Error(),500); return }
	jsonOut(w,map[string]any{"id":id,"name":name,"software":software,"country":country,"description":description})
}

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	var bbs,endpoints,probes int64
	_ = s.db.QueryRowContext(r.Context(),`SELECT count(*) FROM bbs`).Scan(&bbs)
	_ = s.db.QueryRowContext(r.Context(),`SELECT count(*) FROM endpoint`).Scan(&endpoints)
	_ = s.db.QueryRowContext(r.Context(),`SELECT count(*) FROM probe_result`).Scan(&probes)
	jsonOut(w,map[string]int64{"bbs":bbs,"endpoints":endpoints,"probes":probes})
}
