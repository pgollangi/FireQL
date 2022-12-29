package main

import (
	"errors"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/olekukonko/tablewriter"
	"github.com/pgollangi/fireql"
	"github.com/spf13/cobra"
	"os"
)

// Version is the version for netselect
var Version string

// Build holds the date bin was released
var Build string

var RootCmd = &cobra.Command{
	Use:           "fireql",
	Short:         "FireQL: Query Firestore using SQL syntax.",
	Long:          `FireQL is Go library and interactive CLI tool to query Google Firestore resources using SQL syntax.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	Run:           runCommand,
	Version:       fmt.Sprintf("%s (%s)\n", Version, Build),
}

func main() {
	RootCmd.Flags().StringP("project", "p", "", "Id of the GCP project")
	RootCmd.Flags().StringP("service-account", "s", "", "Path to service account file to authenticate with Firestore")
	RootCmd.SetVersionTemplate(fmt.Sprintf("fireql version %s (%s)\nFor more info: github.com/pgollangi/FireQL\n", Version, Build))

	err := RootCmd.MarkFlagRequired("project")
	if err != nil {
		printError(err)
		return
	}
	err = RootCmd.Execute()
	if err != nil {
		printError(err)
	}

}

type CmdContext struct {
	fsQuery *fireql.FireQL
}

var ctx *CmdContext

func runCommand(cmd *cobra.Command, args []string) {
	projectId, err := cmd.Flags().GetString("project")
	if err != nil {
		printError(err)
		return
	}

	var serviceAccount string

	serviceAccountFile, err := cmd.Flags().GetString("service-account")
	if serviceAccountFile != "" {

		if err != nil {
			printError(errors.New(fmt.Sprintf("service-account: %s", err)))
			return
		}

		serviceAccountData, err := os.ReadFile(serviceAccountFile)
		if err != nil {
			printError(errors.New(fmt.Sprintf("service-account: %s", err)))
			return
		}
		serviceAccount = string(serviceAccountData)
	}
	var fsQuery *fireql.FireQL
	if serviceAccount == "" {
		fsQuery, err = fireql.NewFireQL(projectId)
	} else {
		fsQuery, err = fireql.NewFireQLWithServiceAccountJSON(projectId, serviceAccount)
	}
	if err != nil {
		printError(err)
		return
	}

	ctx = &CmdContext{fsQuery: fsQuery}

	fmt.Println("Welcome! Use SQL to query Firestore.\nUse Ctrl+D, type \"exit\" to exit.\nVisit github.com/pgollangi/FireQL for more details.")
	initPrompt(fsQuery)
}

func initPrompt(query *fireql.FireQL) {
	p := prompt.New(
		executor,
		completer,
		prompt.OptionPrefix("fireql>"),
		prompt.OptionTitle("fireql"),
		prompt.OptionSetExitCheckerOnInput(exitChecker),
		prompt.OptionAddKeyBind())
	p.Run()
}

func printResult(result *fireql.QueryResult) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(result.Fields)

	for _, row := range result.Records {
		tRow := make([]string, len(result.Fields))
		for idx, field := range result.Fields {
			tRow[idx] = fmt.Sprintf("%v", row[field])
		}
		table.Append(tRow)
	}
	table.Render()
}

func printError(err error) {
	fmt.Printf("error: %s \n", err.Error())
}

func executor(q string) {
	if q == "exit" {
		os.Exit(0)
		return
	}
	result, err := ctx.fsQuery.Execute(q)
	if err != nil {
		printError(err)
	} else {
		printResult(result)
	}
}
func completer(d prompt.Document) []prompt.Suggest {
	var s []prompt.Suggest
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func exitChecker(in string, breakline bool) bool {
	return breakline && in == "exit"
}
