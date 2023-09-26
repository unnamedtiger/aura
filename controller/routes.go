package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
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
