package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

type Id struct {
	s string
}

func ParseId(s string) *Id {
	if len(s) == 0 {
		return nil
	}
	return &Id{s}
}

func (id Id) ToObjectId() bson.ObjectId {
	return bson.ObjectIdHex(id.s)
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

type MongoConf struct {
	Host       string
	Database   string
	Collection string
}

type Entry struct {
	Id      bson.ObjectId `bson:"_id"`
	Content Content
}

func CreateInsertEntry(content Content) Entry {
	id := bson.NewObjectId()
	return Entry{id, content}
}

func CreateUpdateEntry(id Id, content Content) Entry {
	return Entry{id.ToObjectId(), content}
}

func MongoStore(conf MongoConf, cmd chan Cmd) {
	session, err := mgo.Dial(conf.Host)
	if err != nil {
		panic(err)
	}
	defer session.Close()
	coll := session.DB(conf.Database).C(conf.Collection)

	for {
		c := <-cmd
		if cInsert, ok := c.(CmdInsert); ok {
			entry := CreateInsertEntry(cInsert.Content)
			err := coll.Insert(entry)
			if err != nil {
				c.Out() <- nil // TODO send a real error
				continue
			}
			entry.Content["id"] = entry.Id
			c.Out() <- entry.Content
			continue
		}
		if cUpdate, ok := c.(CmdUpdate); ok {
			fmt.Printf("Update document at id %v\n", cUpdate.Id)
			entry := CreateUpdateEntry(cUpdate.Id, cUpdate.Content)
			err := coll.UpdateId(entry.Id, entry)
			if err != nil {
				c.Out() <- nil // TODO send a real error
				continue
			}
			c.Out() <- entry.Content
			continue
		}
		if cSelect, ok := c.(CmdSelect); ok {
			var entry Entry
			var id bson.ObjectId = cSelect.Id.ToObjectId()
			err := coll.FindId(id).One(&entry)
			if err != nil {
				c.Out() <- nil // TODO send a real error
				continue
			}
			entry.Content["id"] = entry.Id
			c.Out() <- entry.Content
			continue
		}
	}
}

type JSONHandler struct {
	Prefix string
	Cmd    chan Cmd
}

func checkId(id *Id) error {
	if !bson.IsObjectIdHex(id.s) {
		return fmt.Errorf("Invalid id: '%v'", id.s)
	}
	return nil
}

func (j *JSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	out := make(chan Content)
	id := ParseId(strings.TrimPrefix(r.URL.Path, j.Prefix))
	if id != nil {
		if err := checkId(id); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\":\"%v\"}\n", err.Error())
			return
		}
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
		if id == nil {
			j.Cmd <- CmdInsert{content, out}
		} else {
			j.Cmd <- CmdUpdate{*id, content, out}
		}
	case "GET":
		fallthrough
	default:
		if id == nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\":\"Missing id\"}\n")
			return
		}
		j.Cmd <- CmdSelect{*id, out}
	}
	res := <-out
	if res == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"error\":\"Unknown id: '%v'\"}\n", *id)
		return
	}

	enc := json.NewEncoder(w)
	enc.Encode(res)
}

type Person struct {
	Name  string
	Phone string
}

func main() {
	httpHost := flag.String("h", "127.0.0.1", "The host to bind")
	httpPort := flag.Int("p", 8080, "The port to listen")
	mongoHost := flag.String("m", "localhost", "The mongo host(:port) (list)")
	mongoDatabase := flag.String("d", "jsonstore", "The mongo database")
	mongoCollection := flag.String("c", "jsons", "The mongo collection")
	flag.Parse()

	prefix := "/json/"

	cmd := make(chan Cmd)
	mongoConf := MongoConf{*mongoHost, *mongoDatabase, *mongoCollection}
	go MongoStore(mongoConf, cmd)

	http.Handle(prefix, &JSONHandler{prefix, cmd})

	hostPort := fmt.Sprintf("%s:%d", *httpHost, *httpPort)

	log.Fatal(http.ListenAndServe(hostPort, nil))
}
