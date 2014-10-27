package main

/*

TODO

- stores in a real DB :-D
- customize HTTP port

*/

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

type CmdInsert struct {
	Content Content
	out     chan Content
}

func (c CmdInsert) Action() Action {
	return INSERT
}

func (c CmdInsert) Out() chan Content {
	return c.out
}

type CmdSelect struct {
	Id  Id
	out chan Content
}

func (c CmdSelect) Action() Action {
	return SELECT
}

func (c CmdSelect) Out() chan Content {
	return c.out
}

type CmdUpdate struct {
	Id      Id
	Content Content
	out     chan Content
}

func (c CmdUpdate) Action() Action {
	return UPDATE
}

func (c CmdUpdate) Out() chan Content {
	return c.out
}

type Cmd interface {
	Action() Action
	Out() chan Content
}

func InMemoryStore(cmd chan Cmd) {
	store := make(map[Id]Content)
	for {
		c := <-cmd
		if cInsert, ok := c.(CmdInsert); ok {
			id := genId()
			fmt.Printf("Store document at id %v\n", id)
			cInsert.Content["id"] = id
			store[id] = cInsert.Content
			c.Out() <- cInsert.Content
			continue
		}
		if cUpdate, ok := c.(CmdUpdate); ok {
			fmt.Printf("Update document at id %v\n", cUpdate.Id)
			cUpdate.Content["id"] = cUpdate.Id
			store[cUpdate.Id] = cUpdate.Content
			c.Out() <- cUpdate.Content
			continue
		}
		if cSelect, ok := c.(CmdSelect); ok {
			c.Out() <- store[cSelect.Id]
			continue
		}
	}
}

type JSONHandler struct {
	Prefix string
	Cmd    chan Cmd
}

func (j *JSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	out := make(chan Content)
	id := Id(strings.TrimPrefix(r.URL.Path, j.Prefix))
	cId := &id
	if len(id) == 0 {
		cId = nil
	}
	switch r.Method {
	case "POST":
		var content Content
		// parse JSON
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&content)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\":\"Can't parse body\"}\n")
			return
		}
		if cId == nil {
			j.Cmd <- CmdInsert{content, out}
		} else {
			j.Cmd <- CmdUpdate{*cId, content, out}
		}
	case "GET":
		fallthrough
	default:
		j.Cmd <- CmdSelect{*cId, out}
	}
	res := <-out
	if res == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"error\":\"Unknown id: '%v'\"}\n", *cId)
		return
	}

	enc := json.NewEncoder(w)
	enc.Encode(res)
}

func main() {
	prefix := "/json/"

	cmd := make(chan Cmd)
	go InMemoryStore(cmd)

	// initializating of the inMemory DB with fake data
	out := make(chan Content)
	content := Content(map[string]interface{}{
		"id":   "0",
		"test": "oui",
	})
	cmd <- CmdInsert{content, out}
	<-out
	// -

	http.Handle(prefix, &JSONHandler{prefix, cmd})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
