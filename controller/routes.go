package main

import (
	"html/template"
	"log"
	"net/http"
)

var templates *template.Template

func RouteRoot(w http.ResponseWriter, r *http.Request) {
	type data struct {
		Projects []Project
	}
	var err error
	d := data{}
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
