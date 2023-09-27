package main

import (
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"sort"
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
	for _, entityKey := range entityKeys {
		entityList, err := FindEntities(project.Id, entityKey, 11)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		entityMore[entityKey] = len(entityList) > 10
		if len(entityList) > 10 {
			entityList = entityList[:10]
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
