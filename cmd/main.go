package main

import (
	"github.com/pgollangi/fireql"
)

func main() {
	//query, _ := fireql.NewFireQL("f5xc-eng-g-ept-d-globl-1e42")
	//r, err := query.Execute("Select Id as ID, GitLabId, `Group.Name` as GroupName from projects LIMIT 1")
	//fmt.Printf("Result %v. %v", r, err)

	query, err := fireql.NewFireQL("GCP_PROJECT_ID")
	if err != nil {
		panic(err)
	}
	result, err := query.
		Execute("Select Email, FullName as Name, `Address.City` as City from users LIMIT 10")
	if err != nil {
		panic(err)
	}
	_ = result
}
