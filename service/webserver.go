package service

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"go.mongodb.org/mongo-driver/mongo"
)

type Server struct {
	ctx       context.Context
	mdbClient *mongo.Client
	rootDir   string
}

func NewWebServer(ctx context.Context, mdbClient *mongo.Client) *http.ServeMux {
	ret := &Server{
		ctx:       ctx,
		mdbClient: mdbClient,
		rootDir:   "/home/dgottlieb/viam/bfserver",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ret.helloHandler)
	mux.HandleFunc("/robot_part_search", ret.robotPartSearchHandler)
	return mux
}

func (server *Server) robotPartSearchHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error parsing template: %v", err)
		return
	}

	parts := searchForPartByName(server.ctx, server.mdbClient, r.Form.Get("robotName"))
	tmplPath := filepath.Join(server.rootDir, "service", "templates", "robotPartSearch.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error parsing template: %v", err)
		return
	}

	tmpl.Execute(w, struct {
		Search     string
		RobotParts []RobotPart
	}{
		r.Form.Get("robotName"),
		parts,
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error executing template: %v", err)
		return
	}
}

func (server *Server) helloHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s %s", r.Method, r.URL.Path)

	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	funcMap := template.FuncMap{
		"isEven": func(a int) bool {
			return a%2 == 0
		},
	}

	// Parse template from file
	tmplPath := filepath.Join(server.rootDir, "service", "templates", "index.html")
	tmpl, err := template.New("index.html").Funcs(funcMap).ParseFiles(tmplPath)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error parsing template: %v", err)
		return
	}

	// Data to pass to template
	data := struct {
		Name    string
		Numbers []int
	}{
		"Gopher",
		[]int{1, 2, 3, 4},
	}

	// Execute template
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error executing template: %v", err)
		return
	}
}
