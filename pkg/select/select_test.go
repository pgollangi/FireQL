package _select

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pgollangi/fireql/pkg/util"
	"io"
	"log"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"cloud.google.com/go/firestore"
)

type TestExpect struct {
	query   string
	columns []string
	length  string
	records [][]interface{}
}

const FirestoreEmulatorHost = "FIRESTORE_EMULATOR_HOST"

var selectTests = []TestExpect{
	TestExpect{
		query:   "select * from users",
		columns: []string{"id", "email", "username", "address", "name"},
		length:  "21",
	},
	TestExpect{
		query:   "select id, email, address from users",
		columns: []string{"id", "email", "address"},
		length:  "21",
	},
	TestExpect{
		query:   "select id, email, address from users limit 5",
		columns: []string{"id", "email", "address"},
		length:  "5",
	},
}

func newFirestoreTestClient(ctx context.Context) *firestore.Client {
	client, err := firestore.NewClient(ctx, "test")
	if err != nil {
		log.Fatalf("firebase.NewClient err: %v", err)
	}

	return client
}

func TestMain(m *testing.M) {
	// command to start firestore emulator
	cmd := exec.Command("gcloud", "beta", "emulators", "firestore", "start", "--host-port=localhost")

	// this makes it killable
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// we need to capture it's output to know when it's started
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	defer stderr.Close()

	// start her up!
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// ensure the process is killed when we're finished, even if an error occurs
	// (thanks to Brian Moran for suggestion)
	var result int
	defer func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		os.Exit(result)
	}()

	// we're going to wait until it's running to start
	var wg sync.WaitGroup
	wg.Add(1)

	// by starting a separate go routine
	go func() {
		// reading it's output
		buf := make([]byte, 256, 256)
		for {
			n, err := stderr.Read(buf[:])
			if err != nil {
				// until it ends
				if err == io.EOF {
					break
				}
				log.Fatalf("reading stderr %v", err)
			}

			if n > 0 {
				d := string(buf[:n])

				// only required if we want to see the emulator output
				log.Printf("%s", d)

				// checking for the message that it's started
				if strings.Contains(d, "Dev App Server is now running") {
					wg.Done()
				}

				// and capturing the FIRESTORE_EMULATOR_HOST value to set
				pos := strings.Index(d, FirestoreEmulatorHost+"=")
				if pos > 0 {
					host := d[pos+len(FirestoreEmulatorHost)+1 : len(d)-1]
					os.Setenv(FirestoreEmulatorHost, host)
				}
			}
		}
	}()

	// wait until the running message has been received
	wg.Wait()

	ctx := context.Background()
	users := newFirestoreTestClient(ctx).Collection("users")

	usersDataRaw, _ := os.ReadFile("../../test/data/users.json")
	var usersData []map[string]interface{}
	json.Unmarshal(usersDataRaw, &usersData)

	for _, user := range usersData {
		users.Doc(fmt.Sprintf("%v", user["id"].(float64))).Set(ctx, user)
	}

	//selectTests = append(selectTests, TestExpect{query: "select * from users", expected: usersData})
	// now it's running, we can run our unit tests
	result = m.Run()
}

func TestSelectQueries(t *testing.T) {
	for _, tt := range selectTests {
		stmt := New(&util.Context{
			ProjectId: "test",
		}, tt.query)
		actual, err := stmt.Execute()
		if err != nil {
			t.Error(err)
		} else {
			sort.Strings(actual.Columns)
			sort.Strings(tt.columns)
			if stringSlicesEqual(actual.Columns, tt.columns) {
				t.Errorf("QueryResult.Fields(%v): expected %v, actual %v", tt.query, tt.columns, actual.Columns)
			}
			if tt.length != "" && len(actual.Records) != first(strconv.Atoi(tt.length)) {
				t.Errorf("len(QueryResult.Records)(%v): expected %v, actual %v", tt.query, len(actual.Records), tt.length)
			}
			if tt.records != nil && !reflect.DeepEqual(actual.Records, tt.records) {
				t.Errorf("QueryResult.Records(%v): expected %v, actual %v", tt.query, tt.records, actual.Records)
			}
		}
	}
}

func first(n int, _ error) int {
	return n
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
