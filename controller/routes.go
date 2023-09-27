package main

import (
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed static/*
var staticData embed.FS

//go:embed templates/*
var templateData embed.FS

var templates *template.Template

func RouteRoot(w http.ResponseWriter, r *http.Request) {
	type data struct {
		Projects []Project
		Title    string
	}
	var err error
	d := data{Title: "All Projects"}
	d.Projects, err = LoadProjects()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	err = templates.ExecuteTemplate(w, "projects.html", d)
	if err != nil {
		log.Println(err)
	}
}

var slugRegex = regexp.MustCompile("^[0-9A-Za-z-_:]{1,260}$")

func RouteProject(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/p/")
	slashes := strings.Count(p, "/")
	if slashes == 0 {
		RouteProjectMain(w, r)
	} else if slashes == 1 {
		RouteProjectKey(w, r)
	} else if slashes == 2 {
		RouteProjectKeyVal(w, r)
	} else {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
}

func RouteProjectKey(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/p/"), "/")
	if len(parts) != 2 {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	projectSlug := parts[0]
	entityKey := parts[1]
	if !slugRegex.MatchString(projectSlug) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !slugRegex.MatchString(entityKey) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	project, err := FindProjectBySlug(projectSlug)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	query := r.URL.Query()
	before := int64(0)
	if query.Has("before") {
		beforeString := query.Get("before")
		before, err = strconv.ParseInt(beforeString, 10, 64)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	limit := 10
	entityList, err := FindEntities(project.Id, entityKey, before, int64(limit+1))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	more := len(entityList) > limit
	if len(entityList) > limit {
		entityList = entityList[:limit]
	}

	older := int64(0)
	if more {
		older = entityList[len(entityList)-1].Created.Unix()
	}

	type data struct {
		Entities    []Entity
		EntityKey   string
		Older       int64
		ProjectName string
		ProjectSlug string
		Title       string
	}
	d := data{Entities: entityList, EntityKey: entityKey, Older: older, ProjectName: project.Name, ProjectSlug: project.Slug, Title: project.Name}
	err = templates.ExecuteTemplate(w, "projectKey.html", d)
	if err != nil {
		log.Println(err)
	}
}

func RouteProjectKeyVal(w http.ResponseWriter, r *http.Request) {

}

func RouteProjectMain(w http.ResponseWriter, r *http.Request) {
	projectSlug := strings.TrimPrefix(r.URL.Path, "/p/")
	if !slugRegex.MatchString(projectSlug) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	project, err := FindProjectBySlug(projectSlug)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	entityKeys, err := FindEntityKeysByProjectId(project.Id)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	sort.Strings(entityKeys)
	entities := map[string][]Entity{}
	entityMore := map[string]bool{}
	limit := 10
	for _, entityKey := range entityKeys {
		entityList, err := FindEntities(project.Id, entityKey, 0, int64(limit+1))
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		entityMore[entityKey] = len(entityList) > limit
		if len(entityList) > limit {
			entityList = entityList[:limit]
		}
		entities[entityKey] = entityList
	}

	type data struct {
		Entities    map[string][]Entity
		EntityKeys  []string
		EntityMore  map[string]bool
		ProjectName string
		ProjectSlug string
		Title       string
	}
	d := data{Entities: entities, EntityKeys: entityKeys, EntityMore: entityMore, ProjectName: project.Name, ProjectSlug: project.Slug, Title: project.Name}
	err = templates.ExecuteTemplate(w, "project.html", d)
	if err != nil {
		log.Println(err)
	}
}
