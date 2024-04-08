package main

import (
	"bufio"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"sort"
	"sync"
	"time"
)

var (
	Version   = "0.1.0"
	Revision  = "unknown"
	BuildTags = ""
)

const (
	DefaultPort          = 9494
	DefaultListDirectory = "lists"
)

var (
	listMu     sync.RWMutex
	listBuffer map[string]map[string]struct{}
	updateChan = make(chan struct{}, 2)

	listTemplate  *template.Template
	indexTemplate *template.Template
)

type TemplateContext struct {
	Title string
	Items []string
	Lists []string
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"version": %q, "revision": %q, "buildTags": %q}`, Version, Revision, BuildTags)))
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/list/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := path.Base(r.URL.Path)

	// FIXME: limit list buffer size
	listMu.RLock()
	list, ok := listBuffer[name]
	listMu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		r.ParseForm()
		listMu.Lock()
		for _, item := range r.PostForm["add"] {
			if item == "" {
				continue
			}
			list[item] = struct{}{}
		}
		for _, item := range r.PostForm["remove"] {
			delete(list, item)
		}
		listMu.Unlock()
	}

	listMu.Lock()
	listBuffer[name] = list
	listMu.Unlock()

	if r.Method == http.MethodGet || r.Method == http.MethodPost {
		var items []string
		for item := range list {
			items = append(items, item)
		}
		sort.Strings(items)

		// FIXME: read from dir
		var lists []string
		listMu.RLock()
		for list := range listBuffer {
			lists = append(lists, list)
		}
		listMu.RUnlock()
		sort.Strings(lists)

		data := TemplateContext{
			Title: name,
			Items: items,
			Lists: lists,
		}
		err := listTemplate.Execute(w, data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "unable to execute list template"}`))
			return
		}
	}

	go func() {
		select {
		case updateChan <- struct{}{}:
		default:
		}
	}()
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		r.ParseForm()
		listMu.Lock()
		for _, item := range r.PostForm["add"] {
			if item == "" {
				continue
			}
			listBuffer[item] = make(map[string]struct{})
		}
		listMu.Unlock()
	}

	if r.Method == http.MethodGet || r.Method == http.MethodPost {
		var lists []string
		listMu.RLock()
		for list := range listBuffer {
			lists = append(lists, list)
		}
		listMu.RUnlock()
		sort.Strings(lists)

		data := TemplateContext{
			Lists: lists,
		}
		err := indexTemplate.Execute(w, data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "unable to execute index template"}`))
			return
		}
	}
}

func readLists(listDirectory string) (map[string]map[string]struct{}, error) {
	buffer := make(map[string]map[string]struct{})
	entries, err := os.ReadDir(listDirectory)
	if err != nil {
		return nil, fmt.Errorf("read list directory: %w", err)
	}

	for _, entry := range entries {
		list := make(map[string]struct{})
		f, err := os.Open(path.Join(listDirectory, entry.Name()))
		defer f.Close()
		if err != nil {
			return nil, fmt.Errorf("open list file: %w", err)
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			list[scanner.Text()] = struct{}{}
		}
		buffer[entry.Name()] = list
	}

	return buffer, nil
}

func updateFiles(listDirectory string, buffer map[string]map[string]struct{}) error {
	for name, list := range buffer {
		filename := fmt.Sprintf("%s/%s.tmp", listDirectory, name)
		f, err := os.Create(filename)
		defer f.Close()
		if err != nil {
			return fmt.Errorf("create list file: %w", err)
		}

		for item := range list {
			fmt.Fprintln(f, item)
		}

		listFile := path.Join(listDirectory, name)
		err = os.Rename(filename, listFile)
		if err != nil {
			return fmt.Errorf("rename list file: %w", err)
		}
	}

	return nil
}

func main() {
	port := fmt.Sprintf(":%d", DefaultPort)
	fs := http.FileServer(http.Dir("./assets"))

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", okHandler)
	mux.HandleFunc("/list/", listHandler)
	mux.Handle("/asset/", http.StripPrefix("/asset/", fs))
	mux.HandleFunc("/", indexHandler)

	var err error
	listMu.Lock()
	listBuffer, err = readLists(DefaultListDirectory)
	listMu.Unlock()
	if err != nil {
		fmt.Println("ERR: read lists:", err)
		os.Exit(1)
	}

	indexTemplate, err = template.ParseFiles("templates/index.tmpl")
	if err != nil {
		fmt.Println("ERR: parse index template:", err)
		os.Exit(1)
	}

	listTemplate, err = template.ParseFiles("templates/list.tmpl")
	if err != nil {
		fmt.Println("ERR: parse list template:", err)
		os.Exit(1)
	}

	go func() {
		for {
			<-updateChan
			listMu.RLock()
			err = updateFiles(DefaultListDirectory, listBuffer)
			listMu.RUnlock()
			if err != nil {
				fmt.Println("ERR: update files:", err)
			}
			<-time.After(5 * time.Second)
		}
	}()

	srv := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	fmt.Printf("Listening at %s\n", port)
	fmt.Println(srv.ListenAndServe())
}
