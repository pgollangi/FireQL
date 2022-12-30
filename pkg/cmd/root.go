package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/olekukonko/tablewriter"
	"github.com/pgollangi/fireql"
	"github.com/pgollangi/fireql/pkg/util"
	"github.com/spf13/cobra"
	"os"
)

// Version is the version for fireql
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

func init() {
	RootCmd.Flags().StringP("project", "p", "", "Required. Id of the GCP project")
	RootCmd.Flags().StringP("service-account", "s", "", "Path to service account file to authenticate with Firestore")
	RootCmd.Flags().IntP("limit", "l", 100, "Default limit to apply on SELECTed results. Set `0` to result unlimited.")

	RootCmd.SetVersionTemplate(fmt.Sprintf("fireql version %s (%s)\nFor more info: github.com/pgollangi/FireQL\n", Version, Build))

	err := RootCmd.MarkFlagRequired("project")
	if err != nil {
		panic(err)
	}
}

func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		printError(err)
	}
}

type Context struct {
	fsQuery *fireql.FireQL
}

var ctx *Context

func runCommand(cmd *cobra.Command, args []string) {
	projectId, err := cmd.Flags().GetString("project")
	if err != nil {
		printError(err)
		return
	}

	var options []fireql.Option

	serviceAccountFile, err := cmd.Flags().GetString("service-account")
	if err != nil {
		printError(errors.New(fmt.Sprintf("service-account: %s", err)))
		return
	}
	if serviceAccountFile != "" {
		serviceAccount, err := os.ReadFile(serviceAccountFile)
		if err != nil {
			printError(errors.New(fmt.Sprintf("service-account: %s", err)))
			return
		}
		options = append(options, fireql.OptionServiceAccount(string(serviceAccount)))
	}

	defaultLimit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		printError(errors.New(fmt.Sprintf("limit: %s", err)))
		return
	}
	if defaultLimit > 0 {
		options = append(options, fireql.OptionDefaultLimit(defaultLimit))
	}

	fsQuery, err := fireql.New(projectId, options...)
	if err != nil {
		printError(err)
		return
	}

	ctx = &Context{fsQuery: fsQuery}

	fmt.Println("Welcome! Use SQL to query Firestore.\nUse Ctrl+D, type \"exit\" to exit.\nVisit github.com/pgollangi/FireQL for more details.")
	initPrompt()
}

func initPrompt() {
	p := prompt.New(
		executor,
		completer,
		prompt.OptionPrefix("fireql>"),
		prompt.OptionTitle("fireql"),
		prompt.OptionSetExitCheckerOnInput(exitChecker),
		prompt.OptionAddKeyBind())
	p.Run()
}

func printResult(result *util.QueryResult) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(result.Columns)

	for _, row := range result.Records {
		tRow := make([]string, len(result.Columns))
		for idx, val := range row {
			switch cellVal := val.(type) {
			case map[string]interface{}:
				jsonVal, err := json.Marshal(cellVal)
				if err == nil {
					val = string(jsonVal)
				} else {
					val = errors.New("error converting map to JSON")
				}
			}
			tRow[idx] = fmt.Sprintf("%v", val)
		}
		table.Append(tRow)
	}
	table.Render()
	fmt.Printf("(%d rows)\n", len(result.Records))
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
