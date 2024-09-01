package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alarbada/go-instantdb"
)

func main() {
	switch os.Args[1] {
	case "basic":
		basic()
	default:
	}

}

type Todo struct {
	Title string `json:"title"`
}

func basic() {
	client := instantdb.NewClient(
		os.Getenv("APP_ID"),
		os.Getenv("SECRET"),
	)

	// client.SetDebug()

	result, err := client.Query(instantdb.Object{"todos": struct{}{}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(result))
}
