package main

import (
	"database/sql"
	"log"
)

var db *sql.DB

type Project struct {
	Id   int64
	Name string
	Slug string
}

func LoadProjects() ([]Project, error) {
	rows, err := db.Query("SELECT id, name, slug FROM projects ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	results := []Project{}
	for rows.Next() {
		var id sql.NullInt64
		var name sql.NullString
		var slug sql.NullString
		err = rows.Scan(&id, &name, &slug)
		if err != nil {
			return nil, err
		}
		results = append(results, Project{Id: id.Int64, Name: name.String, Slug: slug.String})
	}
	return results, nil
}

func tryExec(query string, args ...any) {
	_, err := db.Exec(query, args...)
	if err != nil {
		log.Fatalln(err)
	}
}

func InitializeDatabase() error {
	tryExec("CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL)")
	return nil
}

func FillDatabaseWithDemoData() error {
	tryExec("INSERT INTO projects (id, name, slug) VALUES (NULL, 'Aura', 'aura')")
	tryExec("INSERT INTO projects (id, name, slug) VALUES (NULL, 'Colors', 'colors')")
	tryExec("INSERT INTO projects (id, name, slug) VALUES (NULL, 'Darke', 'darke')")
	return nil
}
