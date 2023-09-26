package main

import (
	"database/sql"
	"errors"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	var dbDemoVar = flag.Bool("demo", false, "use a demo database")
	flag.Parse()
	dbDemo := dbDemoVar != nil && *dbDemoVar

	dbFilename := "aura.db"
	if dbDemo {
		dbFilename = "aura.demo.db"
	}
	dbExists := true
	_, err := os.Stat(dbFilename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			dbExists = false
		} else {
			log.Fatalln(err)
		}
	}
	log.Printf("Opening database %s...", dbFilename)
	db, err = sql.Open("sqlite", dbFilename)
	if err != nil {
		log.Fatalln(err)
	}
	if !dbExists {
		log.Printf("Initializing database...")
		err = InitializeDatabase()
		if err != nil {
			log.Fatalln(err)
		}
		if dbDemo {
			log.Printf("Filling database with demo data...")
			err = FillDatabaseWithDemoData()
			if err != nil {
				log.Fatalln(err)
			}
		}
	}

	templates = template.Must(template.New("pages").ParseFiles("templates/projects.html"))

	router := http.NewServeMux()
	router.HandleFunc("/", RouteRoot)

	requestLogger := func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("- %s \"%s %s %s\"", r.RemoteAddr, r.Method, r.URL, r.Proto)
			handler.ServeHTTP(w, r)
		})
	}
	log.Println("Serving on port 8420...")
	log.Fatalln(http.ListenAndServe(":8420", requestLogger(router)))
}
