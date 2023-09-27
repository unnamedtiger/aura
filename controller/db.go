package main

import (
	"database/sql"
	"errors"
	"log"
	"time"
)

var ErrNotFound = errors.New("not found")

var db *sql.DB

type Entity struct {
	Id        int64
	ProjectId int64
	Key       string
	Val       string
	Created   time.Time
}

type Project struct {
	Id   int64
	Name string
	Slug string
}

func FindEntities(projectId int64, key string, before int64, limit int64) ([]Entity, error) {
	query := "SELECT id, projectId, key, val, created FROM entities WHERE projectId = ? AND key = ? "
	args := []any{projectId, key}
	if before > 0 {
		query += "AND created < ? "
		args = append(args, before)
	}
	query += "ORDER BY created DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	results := []Entity{}
	for rows.Next() {
		entity, err := ScanEntity(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, entity)
	}
	return results, nil
}

func FindEntityKeysByProjectId(projectId int64) ([]string, error) {
	rows, err := db.Query("SELECT DISTINCT key FROM entities WHERE projectId = ?", projectId)
	if err != nil {
		return nil, err
	}
	results := []string{}
	for rows.Next() {
		var key sql.NullString
		err := rows.Scan(&key)
		if err != nil {
			return nil, err
		}
		results = append(results, key.String)
	}
	return results, nil
}

func FindProjectBySlug(slug string) (Project, error) {
	rows, err := db.Query("SELECT id, name, slug FROM projects WHERE slug = ?", slug)
	if err != nil {
		return Project{}, err
	}
	if rows.Next() {
		project, err := ScanProject(rows)
		if err != nil {
			return Project{}, err
		}
		return project, nil
	}
	return Project{}, ErrNotFound
}

func LoadProjects() ([]Project, error) {
	rows, err := db.Query("SELECT id, name, slug FROM projects ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	results := []Project{}
	for rows.Next() {
		project, err := ScanProject(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, project)
	}
	return results, nil
}

func ScanEntity(rows *sql.Rows) (Entity, error) {
	var id sql.NullInt64
	var projectId sql.NullInt64
	var key sql.NullString
	var val sql.NullString
	var createdTimestamp sql.NullInt64
	err := rows.Scan(&id, &projectId, &key, &val, &createdTimestamp)
	if err != nil {
		return Entity{}, err
	}
	created := time.Unix(createdTimestamp.Int64, 0)
	return Entity{Id: id.Int64, ProjectId: projectId.Int64, Key: key.String, Val: val.String, Created: created}, nil
}

func ScanProject(rows *sql.Rows) (Project, error) {
	var id sql.NullInt64
	var name sql.NullString
	var slug sql.NullString
	err := rows.Scan(&id, &name, &slug)
	if err != nil {
		return Project{}, err
	}
	return Project{Id: id.Int64, Name: name.String, Slug: slug.String}, nil
}

func tryExec(tx *sql.Tx, query string, args ...any) {
	_, err := tx.Exec(query, args...)
	if err != nil {
		log.Fatalln(err)
	}
}

func InitializeDatabase() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	tryExec(tx, "CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL)")
	tryExec(tx, "CREATE TABLE entities (id INTEGER PRIMARY KEY, projectId INTEGER NOT NULL, key TEXT NOT NULL, val TEXT NOT NULL, created INTEGER NOT NULL, FOREIGN KEY (projectId) REFERENCES projects(id))")
	return tx.Commit()
}

func FillDatabaseWithDemoData() error {
	t := time.Now()
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	tryExec(tx, "INSERT INTO projects (id, name, slug) VALUES (NULL, 'Colors', 'colors')")
	tryExec(tx, "INSERT INTO projects (id, name, slug) VALUES (NULL, 'Darke', 'darke')")
	tryExec(tx, "INSERT INTO projects (id, name, slug) VALUES (NULL, 'Aura', 'aura')")

	for i := 1; i <= 135; i++ {
		dt := t.Add(time.Duration(-(730-i)*24) * time.Hour)
		tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 1, 'rev', ?, ?)", i, dt.Unix())
	}
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '101a8f4a011a138cb6bdd3a9eb3810b6c56421a2f31a21c3a467adfa7d4b0765b7', ?)", t.Add(-1335*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '10ef8fbe2d9c474169eac65aafccf626218a9fd3874b400e3d8c2cb924958e27e7', ?)", t.Add(-1215*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '10c0e255ead9779eba84441610509618f688bf88e79452033f7d5d8e01d1f1c4d1', ?)", t.Add(-1095*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '103b4353161dbc1f05117b14c3e43a6ac665e61616575d07fedaf3b1461f33bf3a', ?)", t.Add(-975*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '103bb753273f6f27322bc8b67b8bb76d68dbf09f6f14d2039d3119cc7740469022', ?)", t.Add(-855*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '10a74cba382f1a2af371cda3c2e5171c4418da481fdc09ce73ecfc88554fc60247', ?)", t.Add(-735*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '1063c5027e880c7ce4ab24dc94b183744dbd00535b0537e80ca9a48009b61decd2', ?)", t.Add(-615*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '1047c2b307a3bf1578775d40fae3f86da8179b55597c65f7b28d0acc6a2c411677', ?)", t.Add(-495*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '100b7037fa04ee41646f7bc1604b2fad881d58b8f519778c0c0debd05895e3a106', ?)", t.Add(-375*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '103b1fa64a689e780acb629ee8f4637810d0756b1462cc1d512748c07abc4515a8', ?)", t.Add(-255*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'commit', '102dd7bb0ad4fa62b01c406b0f9c4cb734bb62f9c9c0f3c5de8bdecff59d1fdd1b', ?)", t.Add(-135*time.Minute).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'mr', '1', ?)", t.Add(-166*time.Hour).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'mr', '2', ?)", t.Add(-168*time.Hour).Unix())
	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'mr', '3', ?)", t.Add(-120*time.Hour).Unix())
	for i := 0; i < 11; i++ {
		dt := t.Add(time.Duration(-(11-i)*24) * time.Hour)
		tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'nightly', ?, ?)", dt.Format("2006-01-02"), dt.Unix())
	}
	return tx.Commit()
}
