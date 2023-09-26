package main

import (
	"html/template"
	"log"
	"net/http"
)

var templates *template.Template

func RouteRoot(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "projects.html", nil)
	if err != nil {
		log.Println(err)
	}
}
