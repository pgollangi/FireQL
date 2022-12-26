package main

import (
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
	RunE:          runCommand,
}

func main() {
	RootCmd.Flags().StringP("project", "p", "", "Id of the GCP project")
	RootCmd.Flags().StringP("service-account", "s", "", "Path to service account file to authenticate with Firestore")
	RootCmd.MarkFlagsRequiredTogether("project")
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}

func runCommand(cmd *cobra.Command, args []string) error {
	if ok, _ := cmd.Flags().GetBool("version"); ok {
		executeVersionCmd()
		return nil
	}

	projectId, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}

	query, err := fireql.NewFireQL(projectId)
	if err != nil {
		return err
	}
	fmt.Println("Please enter query.")
	initPrompt(query)
	return nil
}

func initPrompt(query *fireql.FireQL) error {
	q := prompt.Input("> ", completer)
	result, err := query.Execute(q)
	if err != nil {
		return err
	}
	printResult(result)
	return initPrompt(query)
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

func completer(d prompt.Document) []prompt.Suggest {
	var s []prompt.Suggest
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func executeVersionCmd() {
	fmt.Printf("fireql version %s (%s)\n", Version, Build)
	fmt.Println("For more info: pgollangi.com/FireQL")
}
