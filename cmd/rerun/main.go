package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/VojtechVitek/rerun"
)

var (
	watch flagStringSlice
)

type flagStringSlice []string

func (f *flagStringSlice) String() string {
	return fmt.Sprintf("%v", *f)
}

func (f *flagStringSlice) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func init() {
	flag.Var(&watch, "watch", "Watch directory/file")
}

type argType int

const (
	argNone argType = iota
	argWatch
	argIgnore
	argRun
)

func main() {
	watcher, err := rerun.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go watcher.Watch()
	defer watcher.Close()

	select {}
}
