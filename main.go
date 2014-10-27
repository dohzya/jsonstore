package main

/*

TODO

- stop checking in InMemoryStore()
	- checks in the controller, then pass the right value
	- InMemoryStore should be able to trust the value it takes
		- a datatype per action?
- stores in a real DB :-D
- customize HTTP port

*/

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

type Action int

const (
	INSERT Action = iota
	SELECT
	UPDATE
)

type Id string

var _currentId = 0

func genId() (id Id) {
	id = Id(fmt.Sprintf("%d", _currentId))
	_currentId += 1
	return
}

type Content map[string]interface{}

type Error struct {
	Code    int
	Message string
}

type ContentOrError struct {
	Content *Content
	Error   *Error
}

type Cmd struct {
	Action  Action
	Id      *Id
	Content *Content
	Out     chan ContentOrError
}

func InMemoryStore(cmd chan *Cmd) {
	store := make(map[Id]*Content)
	for {
		c := <-cmd
		var id *Id
		switch c.Action {
		case INSERT:
			gen := genId()
			id = &gen
			fallthrough
		case UPDATE:
			if id == nil {
				id = c.Id
			}
			if id == nil {
				c.Out <- ContentOrError{nil, &Error{400, "Missing id"}}
				continue
			}
			if c.Content == nil {
				c.Out <- ContentOrError{nil, &Error{400, "Empty content"}}
				continue
			}
			fmt.Printf("Store document at id %v\n", *id)
			(*c.Content)["id"] = *id
			store[*id] = c.Content
			c.Out <- ContentOrError{c.Content, nil}
		case SELECT:
			if c.Id == nil {
				c.Out <- ContentOrError{nil, &Error{400, "Missing id"}}
				continue
			}
			content := store[*c.Id]
			if content == nil {
				c.Out <- ContentOrError{nil, &Error{404, fmt.Sprintf("Unknown id: '%v'", *c.Id)}}
				continue
			}
			c.Out <- ContentOrError{content, nil}
		}
	}
}

type JSONHandler struct {
	Prefix string
	Cmd    chan *Cmd
}

func (j *JSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	out := make(chan ContentOrError)
	id := Id(strings.TrimPrefix(r.URL.Path, j.Prefix))
	cId := &id
	if len(id) == 0 {
		cId = nil
	}
	var content Content
	var action Action
	switch r.Method {
	case "POST":
		if cId == nil {
			action = INSERT
		} else {
			action = UPDATE
		}
		// parse JSON
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&content)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\":\"Can't parse body\"}\n")
			return
		}
	case "GET":
		fallthrough
	default:
		action = SELECT
	}
	j.Cmd <- &Cmd{action, cId, &content, out}
	res := <-out
	if res.Error != nil {
		w.WriteHeader(res.Error.Code)
		fmt.Fprintf(w, "{\"error\":\"%v\"}\n", res.Error.Message)
		return
	}

	enc := json.NewEncoder(w)
	enc.Encode(*res.Content)
}

func main() {
	prefix := "/json/"

	cmd := make(chan *Cmd)
	go InMemoryStore(cmd)

	// initializating of the inMemory DB with fake data
	out := make(chan ContentOrError)
	content := Content(map[string]interface{}{
		"id":   "0",
		"test": "oui",
	})
	cmd <- &Cmd{INSERT, nil, &content, out}
	res := <-out
	if res.Error != nil {
		fmt.Fprintln(os.Stderr, "Error during initialization: ", res.Error)
		os.Exit(1)
	}
	// -

	http.Handle(prefix, &JSONHandler{prefix, cmd})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
