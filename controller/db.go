package main

import (
	"database/sql"
	"errors"
	"log"
	"sort"
	"time"
)

var ErrNotFound = errors.New("not found")

var db *sql.DB

const (
	StatusCreated = iota
	StatusStarted
	StatusCancelled
	StatusSucceeded
	StatusFailed

	StatusEnd // this is the last one and it's invalid
)

func jobStatus(status int) string {
	switch status {
	case StatusCreated:
		return "created"
	case StatusStarted:
		return "started"
	case StatusCancelled:
		return "cancelled"
	case StatusSucceeded:
		return "succeeded"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type EntityOrCollection struct {
	Id        int64
	ProjectId int64
	Key       string
	Val       string
	Created   time.Time
}

type Job struct {
	Id            int64
	EntityId      int64
	Name          string
	Status        int
	Created       time.Time
	EarliestStart time.Time
	Started       time.Time
	Ended         time.Time
	Tag           string
	Runner        int64
	ExitCode      int64
}

type Project struct {
	Id   int64
	Name string
	Slug string
}

type Runner struct {
	Id   int64
	Name string
}

func CreateEntity(projectId int64, key string, val string, created time.Time) error {
	_, err := db.Exec("INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, ?, ?, ?, ?)", projectId, key, val, created.Unix())
	return err
}

func CreateJob(entityId int64, name string, created time.Time, earliestStart time.Time, tag string) error {
	_, err := db.Exec("INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, NULL, NULL, ?, NULL, 0)", entityId, name, StatusCreated, created.Unix(), earliestStart.Unix(), tag)
	return err
}

func FindCollection(projectId int64, key string, val string) (EntityOrCollection, error) {
	return findEntityOrCollection("collections", projectId, key, val)
}

func FindCollectionKeysByProjectId(projectId int64) ([]string, error) {
	rows, err := db.Query("SELECT DISTINCT key FROM collections WHERE projectId = ? ORDER BY key ASC", projectId)
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

func FindEntitiesInCollection(collectionId int64, before int64, limit int64) ([]EntityOrCollection, error) {
	query := "SELECT entities.id, entities.projectId, entities.key, entities.val, entities.created FROM entities INNER JOIN collectionsEntities on entities.id = collectionsEntities.entityId WHERE collectionsEntities.collectionId = ? "
	args := []any{collectionId}
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
	results := []EntityOrCollection{}
	for rows.Next() {
		entity, err := ScanEntityOrCollection(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, entity)
	}
	return results, nil
}

func findEntitiesOrCollections(table string, projectId int64, key string, before int64, limit int64) ([]EntityOrCollection, error) {
	query := "SELECT id, projectId, key, val, created FROM " + table + " WHERE projectId = ? AND key = ? "
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
	results := []EntityOrCollection{}
	for rows.Next() {
		entity, err := ScanEntityOrCollection(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, entity)
	}
	return results, nil
}

func FindEntitiesOrCollections(projectId int64, key string, before int64, limit int64) ([]EntityOrCollection, error) {
	entities, err := findEntitiesOrCollections("entities", projectId, key, before, limit)
	if err != nil {
		return nil, err
	}
	if len(entities) > 0 {
		return entities, nil
	}
	collections, err := findEntitiesOrCollections("collections", projectId, key, before, limit)
	if err != nil {
		return nil, err
	}
	return collections, nil
}

func FindEntity(projectId int64, key string, val string) (EntityOrCollection, error) {
	return findEntityOrCollection("entities", projectId, key, val)
}

func findEntityOrCollection(table string, projectId int64, key string, val string) (EntityOrCollection, error) {
	rows, err := db.Query("SELECT id, projectId, key, val, created FROM "+table+" WHERE projectId = ? AND key = ? AND val = ?", projectId, key, val)
	if err != nil {
		return EntityOrCollection{}, err
	}
	if rows.Next() {
		entity, err := ScanEntityOrCollection(rows)
		rows.Close()
		if err != nil {
			return EntityOrCollection{}, err
		}
		return entity, nil
	}
	return EntityOrCollection{}, ErrNotFound
}

func FindEntityKeysByProjectId(projectId int64) ([]string, error) {
	rows, err := db.Query("SELECT DISTINCT key FROM entities WHERE projectId = ? ORDER BY key ASC", projectId)
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

func FindEntityOrCollectionKeysByProjectId(projectId int64) ([]string, error) {
	entityKeys, err := FindEntityKeysByProjectId(projectId)
	if err != nil {
		return nil, err
	}
	collectionKeys, err := FindCollectionKeysByProjectId(projectId)
	if err != nil {
		return nil, err
	}
	if len(entityKeys) == 0 {
		return collectionKeys, nil
	} else if len(collectionKeys) == 0 {
		return entityKeys, nil
	}
	keyMap := map[string]bool{}
	for _, k := range entityKeys {
		keyMap[k] = true
	}
	for _, k := range collectionKeys {
		keyMap[k] = true
	}
	keys := make([]string, 0, len(keyMap))
	for k := range keyMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func FindJobs(entityId int64) ([]Job, error) {
	rows, err := db.Query("SELECT id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode FROM jobs WHERE entityId = ? ORDER BY created ASC", entityId)
	if err != nil {
		return nil, err
	}
	results := []Job{}
	for rows.Next() {
		job, err := ScanJob(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, job)
	}
	return results, nil
}

func FindJobsForRunner(tag string, limit int64) ([]int64, error) {
	rows, err := db.Query("SELECT id FROM jobs WHERE tag = ? ORDER BY created ASC LIMIT ?", tag, limit)
	if err != nil {
		return nil, err
	}
	results := []int64{}
	for rows.Next() {
		var id int64
		err := rows.Scan(&id)
		if err != nil {
			return nil, err
		}
		results = append(results, id)
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
		rows.Close()
		if err != nil {
			return Project{}, err
		}
		return project, nil
	}
	return Project{}, ErrNotFound
}

func FindQueuedJobs(before int64, limit int64) ([]Job, error) {
	query := "SELECT id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode FROM jobs WHERE status = ? "
	args := []any{StatusCreated}
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
	results := []Job{}
	for rows.Next() {
		job, err := ScanJob(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, job)
	}
	return results, nil
}

func FindRunnerByName(name string) (Runner, error) {
	rows, err := db.Query("SELECT id, name FROM runners WHERE name = ?", name)
	if err != nil {
		return Runner{}, err
	}
	if rows.Next() {
		runner, err := ScanRunner(rows)
		rows.Close()
		if err != nil {
			return Runner{}, err
		}
		return runner, nil
	}
	return Runner{}, ErrNotFound
}

func LoadEntity(id int64) (EntityOrCollection, error) {
	rows, err := db.Query("SELECT id, projectId, key, val, created FROM entities WHERE id = ?", id)
	if err != nil {
		return EntityOrCollection{}, err
	}
	if rows.Next() {
		entity, err := ScanEntityOrCollection(rows)
		rows.Close()
		if err != nil {
			return EntityOrCollection{}, err
		}
		return entity, nil
	}
	return EntityOrCollection{}, ErrNotFound
}

func LoadJob(id int64) (Job, error) {
	rows, err := db.Query("SELECT id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode FROM jobs WHERE id = ?", id)
	if err != nil {
		return Job{}, err
	}
	if rows.Next() {
		job, err := ScanJob(rows)
		rows.Close()
		if err != nil {
			return Job{}, err
		}
		return job, nil
	}
	return Job{}, ErrNotFound
}

func LoadProject(id int64) (Project, error) {
	rows, err := db.Query("SELECT id, name, slug FROM projects WHERE id = ?", id)
	if err != nil {
		return Project{}, err
	}
	if rows.Next() {
		project, err := ScanProject(rows)
		rows.Close()
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

func LoadRunner(id int64) (Runner, error) {
	rows, err := db.Query("SELECT id, name FROM runners WHERE id = ?", id)
	if err != nil {
		return Runner{}, err
	}
	if rows.Next() {
		runner, err := ScanRunner(rows)
		rows.Close()
		if err != nil {
			return Runner{}, err
		}
		return runner, nil
	}
	return Runner{}, ErrNotFound
}

func LoadRunners() ([]Runner, error) {
	rows, err := db.Query("SELECT id, name FROM runners ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	results := []Runner{}
	for rows.Next() {
		runner, err := ScanRunner(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, runner)
	}
	return results, nil
}

func ReserveJobForRunner(jobId int64, runnerId int64, now time.Time) (Job, error) {
	res, err := db.Exec("UPDATE jobs SET status = ?, started = ?, runner = ? WHERE id = ? AND status = ?", StatusStarted, now.Unix(), runnerId, jobId, StatusCreated)
	if err != nil {
		return Job{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return Job{}, err
	}
	if rows == 1 {
		return LoadJob(jobId)
	} else {
		return Job{}, ErrNotFound
	}
}

func ScanEntityOrCollection(rows *sql.Rows) (EntityOrCollection, error) {
	var id int64
	var projectId int64
	var key string
	var val string
	var createdTimestamp int64
	err := rows.Scan(&id, &projectId, &key, &val, &createdTimestamp)
	if err != nil {
		return EntityOrCollection{}, err
	}
	created := time.Unix(createdTimestamp, 0)
	return EntityOrCollection{Id: id, ProjectId: projectId, Key: key, Val: val, Created: created}, nil
}

func ScanJob(rows *sql.Rows) (Job, error) {
	var id int64
	var entityId int64
	var name string
	var statusInt int64
	var createdTimestamp int64
	var earliestStartTimestamp int64
	var startedTimestamp sql.NullInt64
	var endedTimestamp sql.NullInt64
	var tag string
	var runnerId sql.NullInt64
	var exitCode int64
	err := rows.Scan(&id, &entityId, &name, &statusInt, &createdTimestamp, &earliestStartTimestamp, &startedTimestamp, &endedTimestamp, &tag, &runnerId, &exitCode)
	if err != nil {
		return Job{}, err
	}
	status := 0
	if statusInt >= 0 && statusInt < StatusEnd {
		status = int(statusInt)
	} else {
		return Job{}, errors.New("invalid status")
	}
	created := time.Unix(createdTimestamp, 0)
	earliestStart := time.Unix(earliestStartTimestamp, 0)
	started := time.Unix(0, 0)
	if startedTimestamp.Valid {
		started = time.Unix(startedTimestamp.Int64, 0)
	}
	ended := time.Unix(0, 0)
	if endedTimestamp.Valid {
		ended = time.Unix(endedTimestamp.Int64, 0)
	}
	runner := int64(0)
	if runnerId.Valid {
		runner = runnerId.Int64
	}
	return Job{Id: id, EntityId: entityId, Name: name, Status: status, Created: created, EarliestStart: earliestStart, Started: started, Ended: ended, Tag: tag, Runner: runner, ExitCode: exitCode}, nil
}

func ScanProject(rows *sql.Rows) (Project, error) {
	var id int64
	var name string
	var slug string
	err := rows.Scan(&id, &name, &slug)
	if err != nil {
		return Project{}, err
	}
	return Project{Id: id, Name: name, Slug: slug}, nil
}

func ScanRunner(rows *sql.Rows) (Runner, error) {
	var id int64
	var name string
	err := rows.Scan(&id, &name)
	if err != nil {
		return Runner{}, err
	}
	return Runner{Id: id, Name: name}, nil
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
	tryExec(tx, "CREATE TABLE runners (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
	tryExec(tx, "CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL)")
	tryExec(tx, "CREATE TABLE entities (id INTEGER PRIMARY KEY, projectId INTEGER NOT NULL, key TEXT NOT NULL, val TEXT NOT NULL, created INTEGER NOT NULL, FOREIGN KEY (projectId) REFERENCES projects(id))")
	tryExec(tx, "CREATE TABLE collections (id INTEGER PRIMARY KEY, projectId INTEGER NOT NULL, key TEXT NOT NULL, val TEXT NOT NULL, created INTEGER NOT NULL, FOREIGN KEY (projectId) REFERENCES projects(id))")
	tryExec(tx, "CREATE TABLE collectionsEntities (id INTEGER PRIMARY KEY, collectionId INTEGER NOT NULL, entityId INTEGER NOT NULL, FOREIGN KEY (collectionId) REFERENCES collections(id), FOREIGN KEY (entityId) REFERENCES entities(id))")
	tryExec(tx, "CREATE TABLE jobs (id INTEGER PRIMARY KEY, entityId INTEGER NOT NULL, name TEXT NOT NULL, status INTEGER NOT NULL, created INTEGER NOT NULL, earliestStart INTEGER NOT NULL, started INTEGER, ended INTEGER, tag TEXT NOT NULL, runner INTEGER, exitCode INTEGER NOT NULL, FOREIGN KEY (entityId) REFERENCES entities(id), FOREIGN KEY (runner) REFERENCES runners(id))")
	// TODO: table for preceding jobs
	return tx.Commit()
}

func FillDatabaseWithDemoData() error {
	t := time.Now()
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	tryExec(tx, "INSERT INTO RUNNERS (id, name) VALUES (NULL, 'buildbox-windows')")
	tryExec(tx, "INSERT INTO RUNNERS (id, name) VALUES (NULL, 'buildbox-linux')")
	tryExec(tx, "INSERT INTO RUNNERS (id, name) VALUES (NULL, 'buildbox-macos')")

	tryExec(tx, "INSERT INTO projects (id, name, slug) VALUES (NULL, 'Colors', 'colors')")
	tryExec(tx, "INSERT INTO projects (id, name, slug) VALUES (NULL, 'Darke', 'darke')")
	tryExec(tx, "INSERT INTO projects (id, name, slug) VALUES (NULL, 'Aura', 'aura')")

	for i := 1; i <= 135; i++ {
		dt := t.Add(time.Duration(-(730-i)*24) * time.Hour)
		tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 1, 'rev', ?, ?)", i, dt.Unix())
		tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, 'build', ?, ?, ?, ?, ?, 'docker,linux', 2, 0)", i, StatusSucceeded, dt.Unix(), dt.Unix(), dt.Add(30*time.Second).Unix(), dt.Add(4*time.Minute).Unix())
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
	tryExec(tx, "INSERT INTO collections (id, projectId, key, val, created) VALUES (NULL, 3, 'mr', '1', ?)", t.Add(-168*time.Hour).Unix())
	tryExec(tx, "INSERT INTO collections (id, projectId, key, val, created) VALUES (NULL, 3, 'mr', '2', ?)", t.Add(-166*time.Hour).Unix())
	tryExec(tx, "INSERT INTO collections (id, projectId, key, val, created) VALUES (NULL, 3, 'mr', '3', ?)", t.Add(-120*time.Hour).Unix())
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 1, 136)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 1, 137)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 1, 138)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 2, 140)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 2, 141)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 2, 142)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 3, 144)")
	tryExec(tx, "INSERT INTO collectionsEntities (id, collectionId, entityId) VALUES (NULL, 3, 145)")
	for i := 0; i < 10; i++ {
		dt := t.Add(time.Duration(-(135 + i*120)) * time.Minute)
		tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, 'build', ?, ?, ?, ?, ?, 'docker,linux', 2, 0)", 136+i, StatusSucceeded, dt.Unix(), dt.Unix(), dt.Add(30*time.Second).Unix(), dt.Add(630*time.Second).Unix())
	}
	for i := 0; i < 11; i++ {
		dt := t.Add(time.Duration(-(11-i)*24) * time.Hour)
		tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 3, 'nightly', ?, ?)", dt.Format("2006-01-02"), dt.Unix())
		tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, 'build', ?, ?, ?, ?, ?, 'docker,linux', 2, 0)", 147+i, StatusSucceeded, dt.Unix(), dt.Unix(), dt.Add(30*time.Second).Unix(), dt.Add(630*time.Second).Unix())
	}
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 0)", 146, "prepare", StatusSucceeded, t.Add(-134*time.Minute).Unix(), t.Add(-134*time.Minute).Unix(), t.Add(-133*time.Minute).Unix(), t.Add(-132*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 1)", 146, "build:linux", StatusFailed, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), t.Add(-128*time.Minute).Unix(), t.Add(-118*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 1)", 146, "build:linux", StatusFailed, t.Add(-108*time.Minute).Unix(), t.Add(-108*time.Minute).Unix(), t.Add(-107*time.Minute).Unix(), t.Add(-97*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 1)", 146, "build:linux", StatusFailed, t.Add(-85*time.Minute).Unix(), t.Add(-85*time.Minute).Unix(), t.Add(-84*time.Minute).Unix(), t.Add(-74*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 1)", 146, "build:linux", StatusFailed, t.Add(-62*time.Minute).Unix(), t.Add(-62*time.Minute).Unix(), t.Add(-61*time.Minute).Unix(), t.Add(-51*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 1)", 146, "build:linux", StatusFailed, t.Add(-39*time.Minute).Unix(), t.Add(-39*time.Minute).Unix(), t.Add(-38*time.Minute).Unix(), t.Add(-28*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 0)", 146, "build:linux", StatusSucceeded, t.Add(-16*time.Minute).Unix(), t.Add(-16*time.Minute).Unix(), t.Add(-15*time.Minute).Unix(), t.Add(-5*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,windows', 1, 0)", 146, "build:windows", StatusSucceeded, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), t.Add(-119*time.Minute).Unix(), t.Add(-107*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', NULL, 0)", 146, "test:linux", StatusCancelled, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), t.Add(-117*time.Minute).Unix(), t.Add(-116*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', 2, 0)", 146, "test:linux", StatusStarted, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), t.Add(-4*time.Minute).Unix(), nil)
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,windows', 1, 0)", 146, "test:windows", StatusSucceeded, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), t.Add(-107*time.Minute).Unix(), t.Add(-88*time.Minute).Unix())
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,macos', NULL, 0)", 146, "build:macos", StatusCreated, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), nil, nil)
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,macos', NULL, 0)", 146, "test:macos", StatusCreated, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), nil, nil)
	tryExec(tx, "INSERT INTO jobs (id, entityId, name, status, created, earliestStart, started, ended, tag, runner, exitCode) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, 'docker,linux', NULL, 0)", 146, "deploy", StatusCreated, t.Add(-132*time.Minute).Unix(), t.Add(-132*time.Minute).Unix(), nil, nil)

	tryExec(tx, "INSERT INTO entities (id, projectId, key, val, created) VALUES (NULL, 2, 'version', 'v1.0.0', ?)", t.Unix())
	return tx.Commit()
}
