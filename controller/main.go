package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	var dbDemoVar = flag.Bool("demo", false, "use a demo database")
	flag.Parse()
	dbDemo := dbDemoVar != nil && *dbDemoVar

	dbFilename := "aura.db"
	if dbDemo {
		dbFilename = fmt.Sprintf("aura.%d.db", time.Now().Unix())
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
	}
	if dbDemo {
		log.Printf("Filling database with demo data...")
		err = FillDatabaseWithDemoData()
		if err != nil {
			log.Fatalln(err)
		}
	}

	templates = template.Must(template.New("pages").ParseFiles("templates/projects.html"))

	http.HandleFunc("/", RouteRoot)

	log.Println("Serving on port 8420...")
	log.Fatalln(http.ListenAndServe(":8420", nil))
}
