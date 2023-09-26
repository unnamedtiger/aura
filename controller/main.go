package main

import (
	"html/template"
	"log"
	"net/http"
)

func main() {
	templates = template.Must(template.New("pages").ParseFiles("templates/projects.html"))

	http.HandleFunc("/", RouteRoot)

	log.Println("Serving on port 8420...")
	log.Fatalln(http.ListenAndServe(":8420", nil))
}
